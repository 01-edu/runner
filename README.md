# Runner

A service to run tests.

- The tests of the students' exercises are done using Docker. Docker is complex (~3M LOC) and slow (`time true` vs `time docker run alpine true`).
- It doesn't need to store persistent data, isn't connected to the database so it therefore suited for a standalone, potentially load-balanced service.

## Installation

```
REGISTRY_PASSWORD=****** ./run.sh
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
    - `Output`: string containing the output of the test or the error executing the test
    - `Ok`: boolean set to `true` if the exit status of the test is zero (success)

### Example

```console
$ go run ./cmd 2>/dev/null &
$ echo mydata > myfile
$ zip archive.zip myfile
$ curl --silent --data-binary @archive.zip 'localhost:8080/alpine?args=sh&args=-c&args=cat+student/myfile' | jq -jr .Output
mydata
$ kill %1
```

## Test environment

- Network is not reachable
- ZIP data is extracted in `/jail/student` (read-only)
- `/jail` is the only writable directory

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
