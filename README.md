# ipverify
Checks proximity of IP addresses against timestamps

## Introduction and Overview
The solution presented here implements the requirements of the IP verify via a single-service that runs in a Docker container, with an ephemeral port exposed outside the container for accessing the service.

The service implements the semantics as described in the spec.  Where there are preceived ambiguities, a justification for the design decison taken will be provided.


## Accessing and running the demo

All external packages are vendored, as required by the assignment.  To start the server, run `docker-compose up` which sends the output to the console, or `docker-compose up -d` to run in detached mode. In the latter case, you can use a tool like Kitematic or use `docker logs` with the container name as such:
```
$ docker  container ls
CONTAINER ID        IMAGE               COMMAND                  CREATED             STATUS              PORTS                     NAMES
b14526d1a55f        ipverify_ipverify   "./ipverify /bin/sh …"   2 minutes ago       Up 2 minutes        0.0.0.0:32772->8080/tcp   ipverify_ipverify_1
$ docker logs ipverify_ipverify_1
{"level":"info","ts":1561237615.7393036,"caller":"ipverify/main.go:87","msg":"Listening for connections","port":8080}
$
```
Notice that running the `docker container ls` (or `docker ps`) shows you the ephemeral port number outside the container, in this case, port 32772. The integration test finds this port number, and also depends on the name of the image to be ipverify_ipverify, which is assured by running it through docker-compose.

**Note: if you would rather run on the same fixed port all the time, simply change the following in docker-compose.yml:**
```
ports:
      - '8080'
```
to

```
ports:
      - '8080:8080'
```

To stop the server, simply type `docker-compose down` or `docker-compose down --rm all` (to remove the image).

To summarize, here are the steps:

* `git clone https://github.com/gdotgordon/ipverify.git` in a place of your choosing (again this code was built and tested with Go 12.1.6, and it uses modules and vendoring through modules.

* `docker-compose up` from the "ipverify" folder

* `docker ps` to find the ephemeral port to connect to the server, e.g "0.0.0.0:32874" means you can use "localhost:32874"
Use a tool like Postman to invoke the endpoints (again, see above to iuse a fixed port number all the time).

* `docker-compose down` from the "ipverify" folder (add `--rmi all`) to remove the image and force a rebuild next time.

* Also, the project uses the Uber Zap logger. This is configured to "production" log level by default, but may be set to "development" to see all the endpoints receiving their requests, by changing IPVERIY_LOG_LEVEL: 'production' to "development" in the docker-compose.yml.  But be warned, debug is very verbose.

Tests
To run the unit tests, you don't need the container running, just run go test -race ./... from the top-level directory.

The unit tests use the "table-driven" approach to writing tests where possible (which is to say almost always).

There is also an integration test under tests/integration that focuses heavily on concurrent execution. You can run that from the root directory by invoking: go test -tags=integration -v -race -count=1 ./tests/integration.  This test runs outside the container, and looks for the ephemeral port by searching for the container by name. If you've started the container through `docker-compose`, this should work fine, as the tests know which server name to look for.

## Key Items and Artifacts and How To Run the IP Verify Service
There are three endpoints:
* `/v1/status` **GET** a liveness status check
* `/v1/verify` **POST** the main endpiont to run the IP verification (with the playload below)
* `/v1/reset` **GET** clears the database (great for testing)

Note unless you explicitly remove the sqlite database file or use the reset endpoint, it will be retained between invocations.

For the `v1/verify` endpoint the main items of interest are the JSON request and response objects.

The request looks like this:
```
{
	"username" : "Bob",
	"unix_timestamp": 1514859999,
	"event_uuid": "85ad929a-db03-4bf4-9541-8f728fa12e42",
	"ip_address": "128.148.252.151"
}
```

The repsonse will have up to three sections, always the first part with info from the current request, and then a previous and subsequent acceess (if either exists).  Note the assignment description said each preceding and subsequent access should contain a field named *suspicious*, but then the example showed those fields outside of those elements.  I found the documented way to be more intuitve for a user, so the suspicious behavior is a field *inside* the preceding and subsequent IP access.
```
{
  {
    "currentGeo":
    "lat": 36.0557,
    "lon": -94.1567,
    "radius": 5
  },
  "precedingIpAccess": {
    "ip": "128.148.252.151",
    "speed": 26,
    "suspicious": false,
    "lat": 41.8244,
    "lon": -71.408,
    "radius": 5,
    "timestamp": 1560578777
  },
  "subsequentIpAccess": {
    "ip": "128.97.27.37",
    "speed": 688,
    "suspicious": true,
    "lat": 34.0648,
    "lon": -118.4414,
    "radius": 10,
    "timestamp": 1560765977
  }
}
```


## The API

Typical HTTP return codes:

200 (OK) for successful requests
400 (Bad Request) if the request is non-conformant to the JSON unmarshal or contains invalid field values, including DB constraint violation, such as using a UUID that already exists in the database
500 (Internal Server Error) typically won't happen unless there is a system failure

Architecture and Code Layout
The code has a main package which starts the HTTP server. This package creates a signal handler which is tied to a context cancel function. This allows for clean shutdown. The main code creates a service object, which is a wrapper around the store package, which uses the sqlite3 database. This service is then passed to the api layer, for use with the mux'ed incoming requests.

As mentioned, Uber Zap logging is used. In a real production product, I would have buried it in a logging interface.

Here is a more-specific roadmap of the packages:

### *types* package
Contains the definitions for the Request and Response types for the various REST invocations and a few other definitons of use throughout the code.

### *api* package
Contains the HTTP handlers for the various endpoints. Primary responsibility is to unmarshal incoming requests, convert them to Go objects, and pass them off to the service layer, get the responses back from the service layer, convert any errors (or not) to appropriate HTTP status codes and send them back to the HTTP layer.

### *service* package
The service implements the Service interface, as SQLiteService.

## External packages used

* github.com/google/uuid - GUID checker and generator: BSD 3-Clause "New" or "Revised" License
* github.com/gorilla/mux - HTTP muxer: BSD 3-Clause "New" or "Revised" License
* github.com/mattn/go-sqlite3 - Sqlite3 DB driver: MIT License
* github.com/oschwald/maxminddb-golang - Maxmind DB reader: ISC License
* github.com/pkg/errors - improved error types: BSD 2-Clause "Simplified" License
* go.uber.org/zap (imports as go.uber.org/zap) - efficient logger: Uber licnese: https://github.com/uber-go/zap/blob/master/LICENSE.txt

Note on packages: for the Haversine function, I found a code snippet from an unacknowledged author on the Go Playground that I made small changes to: https://play.golang.org/p/MZVh5bRWqN.  But to give full credit, I also looked at "github.com/paultag/go-haversine" and "github.com/umahmood/haversine", and saw the implementations were all very close to the Playground sample.  So the code is the result of some combination of the above - in the end I liked my final result the best.
