package runner

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

var (
	ctx = context.Background()
	cli *client.Client

	labels = map[string]string{"org.01-edu.type": "test"}
)

func init() {
	var err error
	cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	expect(nil, err)
}

func expect(target, err error) {
	if err != nil && err != target && !errors.Is(err, target) {
		panic(err)
	}
}

func logDuration(remote string) func(event string) {
	t := time.Now()
	return func(event string) {
		now := time.Now()
		secs := float64(now.Sub(t)) / float64(time.Second)
		log.Println("", remote, "", fmt.Sprintf("%.3fs", secs), "", event)
		t = now
	}
}

var ids = struct {
	sync.Mutex
	m map[string]int
}{m: map[string]int{}}

var updates = struct {
	sync.Mutex
	m map[string]time.Time
}{m: map[string]time.Time{}}

func Run(r *http.Request) ([]byte, bool, error) {
	// Parse URL path, query
	remote := strings.Split(r.Header.Get("X-Forwarded-For"), ", ")[0]
	if remote == "" {
		// No proxy detected, fallback to real IP (IPv4 or IPv6)
		parts := strings.Split(r.RemoteAddr, ":")
		remote = strings.Join(parts[:len(parts)-1], ":") // remove port
	}
	ids.Lock()
	ids.m[remote]++
	remote += "#" + strconv.Itoa(ids.m[remote])
	ids.Unlock()
	defer logDuration(remote)("total")
	logDuration := logDuration(remote)
	image := strings.Trim(r.URL.Path, "/")
	query := r.URL.Query()
	env := query["env"]
	args := query["args"]

	// Read body and convert its ZIP content to a TAR archive
	r.Body = http.MaxBytesReader(nil, r.Body, http.DefaultMaxHeaderBytes)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, false, errors.New(err.Error() + ": is your repository too large?")
	}
	bodyReader := bytes.NewReader(b)
	zipReader, err := zip.NewReader(bodyReader, bodyReader.Size())
	if err != nil {
		return nil, false, err
	}
	var tarBuffer bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuffer)
	defer tarWriter.Close()
	logDuration("read all")
	var prefix string
	{ // Find the unique parent folder if it exists (in order to remove it later)
		rootPaths := map[string]struct{}{}
		for _, file := range zipReader.File {
			if file.FileInfo().IsDir() {
				parent := strings.Split(file.Name, "/")[0]
				rootPaths[parent] = struct{}{}
			}
		}
		if len(rootPaths) == 1 {
			for rootPath := range rootPaths {
				prefix = rootPath
			}
		}
	}
	for _, file := range zipReader.File {
		var isFile bool
		if file.FileInfo().IsDir() {
		} else if file.Mode().IsRegular() {
			isFile = true
		} else {
			continue
		}
		h := &tar.Header{
			Name: strings.TrimPrefix(file.Name, prefix),
			Mode: 0700,
			Uid:  1000,
			Gid:  1000,
		}
		if isFile {
			h.Mode = 0600
			h.Size = file.FileInfo().Size()
		}
		if err := tarWriter.WriteHeader(h); err != nil {
			return nil, false, err
		}
		if isFile {
			fileData, err := file.Open()
			if err != nil {
				return nil, false, err
			}
			_, err = io.Copy(tarWriter, fileData)
			if err != nil {
				fileData.Close()
				return nil, false, err
			}
			fileData.Close()
		}
	}
	logDuration("zip to tar")

	// Refresh Docker image every minute
	updates.Lock()
	if time.Since(updates.m[image]) > time.Minute {
		// Try to pull the image
		err = func() error {
			var options types.ImagePullOptions
			if strings.HasPrefix(image, "docker.01-edu.org/") {
				b, err := json.Marshal(types.AuthConfig{
					Username: "root",
					Password: os.Getenv("REGISTRY_PASSWORD"),
				})
				expect(nil, err)
				options.RegistryAuth = base64.URLEncoding.EncodeToString(b)
			}
			resp, err := cli.ImagePull(ctx, image, options)
			if err != nil {
				return err
			}
			resp.Close()
			return nil
		}()
		if err != nil {
			if _, _, err := cli.ImageInspectWithRaw(ctx, image); err != nil {
				// The image doesn't exist
				updates.Unlock()
				return nil, false, err
			}
			// Pulling the image has failed but an old revision already exists
		}
		log.Println(err)
		updates.m[image] = time.Now()
	}
	updates.Unlock()
	logDuration("image pulled: " + image)

	// Create the volume that will contain the code to test
	volume, err := cli.VolumeCreate(ctx, volume.VolumeCreateBody{Labels: labels})
	if err != nil {
		return nil, false, err
	}

	logError := func(event string, err error) {
		if err != nil {
			event += ": " + err.Error()
		}
		logDuration(event)
	}

	defer func() {
		logError("volume remove", cli.VolumeRemove(ctx, volume.Name, false))
	}()

	// helper to remove a container
	containerRemove := func(id string) {
		logError("container remove", cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{
			RemoveVolumes: true,
			Force:         true,
		}))
	}

	// Create a container whose sole purpose is to copy the TAR archive to the volume
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:  image,
		Labels: labels,
	}, &container.HostConfig{
		Binds: []string{volume.Name + ":/data"},
	}, nil, nil, "")
	if err != nil {
		return nil, false, err
	}
	logDuration("container creation")

	err = cli.CopyToContainer(ctx, resp.ID, "/data", &tarBuffer, types.CopyToContainerOptions{
		CopyUIDGID: true,
	})
	containerRemove(resp.ID)
	if err != nil {
		return nil, false, err
	}
	logDuration("container copy")

	// Create the test container with security contraints
	maxPID := int64(256)
	hostconfig := container.HostConfig{
		LogConfig: container.LogConfig{
			Type: "json-file",
			Config: map[string]string{
				"max-size": "1m",
				"max-file": "2",
			},
		},
		ReadonlyRootfs: true,
		Resources: container.Resources{
			PidsLimit: &maxPID,
			Memory:    500e6,
			NanoCPUs:  2e9,
		},
		NetworkMode: "none",
		Tmpfs:       map[string]string{"/jail": "size=100M,noatime,exec,nodev,nosuid,uid=1000,gid=1000,nr_inodes=5k,mode=1700"},
		Binds:       []string{volume.Name + ":/jail/student:ro"},
	}
	// Redirect a domain name to localhost if needed
	for _, val := range env {
		if strings.HasPrefix(val, "DOMAIN=") {
			hostconfig.ExtraHosts = []string{strings.TrimPrefix(val, "DOMAIN=") + ":127.0.0.1"}
		}
	}
	resp, err = cli.ContainerCreate(ctx, &container.Config{
		Image:      image,
		User:       "1000:1000",
		WorkingDir: "/jail",
		Env: append(env,
			"HOME=/jail",
			"TMPDIR=/jail",
		),
		Labels: labels,
		Cmd:    args,
	}, &hostconfig, nil, nil, "")
	if err != nil {
		return nil, false, err
	}
	logDuration("container creation")

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		containerRemove(resp.ID)
		return nil, false, err
	}
	logDuration("container start")

	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	statusCh, errCh := cli.ContainerWait(ctxTimeout, resp.ID, container.WaitConditionNotRunning)
	var ok bool
	select {
	case err := <-errCh:
		if errors.Is(err, context.DeadlineExceeded) {
			if err := cli.ContainerKill(ctx, resp.ID, "SIGKILL"); err != nil {
				containerRemove(resp.ID)
				return nil, false, err
			}
		}
		containerRemove(resp.ID)
		return nil, false, errors.New("timeout: Did you write an infinite loop? " + err.Error())
	case status := <-statusCh:
		ok = status.StatusCode == 0
		if !ok && status.Error != nil {
			panic(status.Error.Message)
		}
	}
	logDuration("container stop")
	out, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		containerRemove(resp.ID)
		return nil, false, err
	}
	// demux stream, see: https://docs.docker.com/engine/api/v1.41/#operation/ContainerAttach
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, out); err != nil {
		containerRemove(resp.ID)
		return nil, false, err
	}
	logDuration("container logs")
	containerRemove(resp.ID)
	b = buf.Bytes()
	if len(b) > http.DefaultMaxHeaderBytes {
		b = b[:http.DefaultMaxHeaderBytes]
		b = append(b, []byte(" ... TRUNCATED")...)
	}
	return b, ok, nil
}
