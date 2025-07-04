build:
	@go build -o bin/wp-upload .

run: build
	@./bin/wp-upload

format:
	@go run . format

process:
	@go run . process

upload:
	@go run . upload

upload-from:
	@go run . upload $(N)

full:
	@go run . full

# Show help
help:
	@echo "Available commands:"
	@echo "  make format      - Format posts.txt document (first step)"
	@echo "  make process     - Process OG images in posts.txt (stops if errors found)"
	@echo "  make upload      - Upload posts to WordPress from posts.txt"
	@echo "  make upload-from N=X - Resume upload from post X (0-based index)"
	@echo "  make full        - Process OG images and upload (only if no errors)"
	@echo "  make build       - Build the binary"
	@echo "  make help        - Show this help"

test:
	@go test -v ./...