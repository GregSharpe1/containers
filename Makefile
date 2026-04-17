IMAGE_DIRS := $(sort $(patsubst %/Dockerfile,%,$(wildcard */Dockerfile)))

.PHONY: list build run build-all guard-image

list:
	@printf '%s\n' $(IMAGE_DIRS)

guard-image:
	@test -n "$(IMAGE)" || (printf 'IMAGE is required\n' >&2; exit 1)
	@test -f "$(IMAGE)/Dockerfile" || (printf "Unknown image '%s'\n" "$(IMAGE)" >&2; exit 1)

build: guard-image
	docker build -t local/$(IMAGE) ./$(IMAGE)

run: guard-image
	docker run --rm -it local/$(IMAGE)

build-all:
	@set -e; for image in $(IMAGE_DIRS); do $(MAKE) build IMAGE=$$image; done
