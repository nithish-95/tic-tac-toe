all: clean build run

build:
	go build -o bin/tictac .
run: 
	bin/tictac
clean:
	go mod tidy
	rm bin/* || true