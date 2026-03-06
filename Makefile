.PHONY: build test clean run-plan run-apply serve ui ui-build

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

serve: build
	./$(BINARY) serve --addr :8080

ui:
	cd ui && npm run dev

ui-build:
	cd ui && npm run build

lint:
	golangci-lint run ./...
