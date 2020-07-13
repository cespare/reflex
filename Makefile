TAG = $(shell git describe --tags --abbrev=0)
all:
	$(MAKE) test
	$(MAKE) build_reflex

test:
	$(info ************    Testing Reflex    ************)
	docker run --rm -v "$(PWD):/usr/src/myapp" -w /usr/src/myapp golang:1.14 go test 
    $(info ************    Testing Reflex DONE!    ************)

build_reflex:
	$(info ************    Building Reflex  amd64  ************)
	docker run --rm -e "GOOS=linux" -e "GOARCH=amd64" -v "$(PWD):/usr/src/myapp" -w /usr/src/myapp golang:1.14 go build -v -o build/linux/amd64/reflex
	$(info ************    Building Reflex  arm  ************)
	docker run --rm -e "GOOS=linux" -e "GOARCH=arm" -v "$(PWD):/usr/src/myapp" -w /usr/src/myapp golang:1.14 go build -v -o build/linux/arm/reflex
	$(info ************    Building Reflex  arm64  ************)
	docker run --rm -e "GOOS=linux" -e "GOARCH=arm64" -v "$(PWD):/usr/src/myapp" -w /usr/src/myapp golang:1.14 go build -v -o build/linux/arm64/reflex
	$(info ************    Building Reflex  darwin  ************)
	docker run --rm -e "GOOS=darwin" -v "$(PWD):/usr/src/myapp" -w /usr/src/myapp golang:1.14 go build -v -o build/macosx/reflex
	$(info ************    BUILD FINISHED    ************)

build_reflex_docker:
	$(info ************    Building docker container   ************)
	git fetch --all
	docker build -t  reflex:$(TAG) .
    $(info ************    Building docker container DONE!    ************)

.PHONY: test build_reflex build_reflex_docker