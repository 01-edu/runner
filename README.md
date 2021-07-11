# Test runner

A service to run tests.

- The tests of the students' exercises are done using Docker. Docker is complex (~3M LOC) and slow (`time true` vs `time docker run alpine true`).
- It doesn't need to store persistent data, isn't connected to the database so it therefore suited for a standalone, potentially load-balanced service.

## Installation

```
[REGISTRY_PASSWORD=******] ./run.sh
```

Where `REGISTRY_PASSWORD` is the password of our private Docker [registry](https://github.com/01-edu/registry).

## Usage

### Input

- URL
  - Path: the Docker image to use (will be pulled if needed)
  - Query
    - `env`: environment variables
    - `args`: command-line arguments
- Body: a ZIP archive

### Output

- Status:
  - `200 OK`: the test has been executed
  - `400 Bad Request`: the test has not been executed
- Body:
  - JSON object
    - `Output`: string containing the output of the test
    - `Ok`: boolean indicating the exit status of the test

### Example

```console
$ go run . &
$ echo mydata > myfile
$ zip archive.zip myfile
$ curl --silent --data-binary @archive.zip 'localhost:8080/alpine?args=sh&args=-c&args=cat+student/myfile' | jq -r .Output
2021/07/11 16:30:26  [::1]#1  0.000s  read all
2021/07/11 16:30:26  [::1]#1  0.000s  zip to tar
2021/07/11 16:30:26  [::1]#1  0.010s  image pull
2021/07/11 16:30:26  [::1]#1  0.119s  container creation
2021/07/11 16:30:26  [::1]#1  0.184s  container remove
2021/07/11 16:30:26  [::1]#1  0.000s  container copy
2021/07/11 16:30:26  [::1]#1  0.104s  container creation
2021/07/11 16:30:26  [::1]#1  0.503s  container start
2021/07/11 16:30:27  [::1]#1  0.088s  container stop
2021/07/11 16:30:27  [::1]#1  0.007s  container logs
2021/07/11 16:30:27  [::1]#1  0.062s  container remove
2021/07/11 16:30:27  [::1]#1  0.012s  volume remove
2021/07/11 16:30:27  [::1]#1  1.089s  total
mydata

$ kill %1
```

## TODO

- Improve documentation
- Improve code comments
- Benchmarks
- Test invalid cases
  - ZIP
    - empty
    - one file
    - one directory
    - several files
    - several directories
    - [Slip](https://snyk.io/research/zip-slip-vulnerability#go)
    - [Bomb](https://github.com/golang/go/issues/33026)
