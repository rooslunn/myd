APP := myd
SRC := cmd/main.go

all: build compare

# 1. Normal Go build
build-normal:
	GOOS=linux GOARCH=amd64 go build -o $(APP) $(SRC)

# 2. Stripped Go build
build-stripped:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(APP)-stripped $(SRC)

# 3. UPX compressed build (depends on stripped)
build-upx: build-stripped
	upx --best --lzma -o $(APP)-upx $(APP)-stripped

# 4. TinyGo build
build-tinygo:
	tinygo build -o $(APP)-tinygo $(SRC)

# Run all builds
# build: build-normal build-stripped build-upx build-tinygo
build: build-normal build-stripped

# Compare sizes
compare:
	@echo "\n=== Binary sizes ==="
# 	@ls -lh $(APP) $(APP)-stripped $(APP)-upx $(APP)-tinygo | awk '{print $$9, $$5}'
	@ls -lh $(APP) $(APP)-stripped | awk '{print $$9, $$5}'

# Clean up
clean:
	rm -f $(APP) $(APP)-stripped $(APP)-upx $(APP)-tinygo
