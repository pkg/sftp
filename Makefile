.PHONY: integration integration_w_race

integration:
	go test -v ./...
	make -C localfs integration

integration_w_race:
	go test -race -v ./...
	make -C localfs integration
