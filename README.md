## Bet placement application

###Prerequisites
1. Docker and docker-compose are installed
2. Ports: 5432, 8083 are free
3. Golang if you want to run tests

###Running
* To run the application use `docker-compose up` or `make start` commands from root of the project.
* To test the application run it and use `make test` or just `go test ./...`
* Or use `curl -X POST -H "Source-Type: game" -d '{"state": "win", "amount": "10.15", "betId": "some generated identificator"}' http://localhost:8083/bet -v` to make sure it works 