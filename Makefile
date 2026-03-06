.PHONY: build test clean run-plan run-apply

BINARY := dcm
MODULE := github.com/dcm-io/dcm

build:
	go build -o $(BINARY) .

test:
	go test ./...

clean:
	rm -f $(BINARY) *.dcm.state

run-plan: build
	./$(BINARY) plan -f examples/web-app/app.yaml

run-apply: build
	./$(BINARY) apply -f examples/web-app/app.yaml

run-status: build
	./$(BINARY) status -f examples/web-app/app.yaml

run-destroy: build
	./$(BINARY) destroy -f examples/web-app/app.yaml

lint:
	golangci-lint run ./...
