# Installation instructions

## By source

- Make sure you have go version 1.11+ installed
- By default it will use MongoDB as the persistence repository. Unless you change it in the project's configuration, make sure that MongoDB is running and that it has the default configuration.
- Checkout the repository and make sure the root folder of the project is called `deployment-engine`.
- Execute `go build -o deployment-engine` in the root folder
- Run the frontend by executing `./deployment-engine`. The server should be listening in port 8080 unless configured otherwise.

## Docker

- Make sure Docker is installed and running on your system
- Docker-compose is recommended to make running the engine easier
- Execute `docker build -t deployment-engine .`