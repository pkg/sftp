.PHONY: integration integration_w_race

DELAY ?= 0

ifneq ($(DELAY),0)
DELAY_FLAG=-delay $(DELAY)
endif

TAGS=integration,sftp.sync.metrics

integration:
	go test -v $(DELAY_FLAG) -tags=$(TAGS)
	go test ./encoding/... ./internal/...
	make -C localfs integration

integration_w_race:
	go test -race -v $(DELAY_FLAG) -tags=$(TAGS)
	go test -race ./encoding/... ./internal/...
	make -C localfs integration_w_race

COUNT ?= 1
BENCHMARK_PATTERN ?= "."

benchmark:
	go test -v -run=NONE -bench=$(BENCHMARK_PATTERN) -benchmem -count=$(COUNT) $(DELAY_FLAG) -tags=$(TAGS)
	make -C localfs benchmark

benchmark_w_memprofile:
ifneq ($(DELAY),0)
	@echo "memprofile with DELAY produces invalid data" >&2
	@exit 1
endif
	go test -v -run=NONE -bench=$(BENCHMARK_PATTERN) -benchmem -count=$(COUNT) -memprofile memprofile.out -tags=integration
	go tool pprof -sample_index=alloc_space -svg -output=memprofile-space.svg memprofile.out
	go tool pprof -sample_index=alloc_objects -svg -output=memprofile-allocs.svg memprofile.out
	make -C localfs benchmark_w_memprofile
