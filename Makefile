BINARY_NAME=batterycap
INSTALL_DIR=$(HOME)/.local/bin

.PHONY: all build install clean run help

all: build

build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) .

install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_DIR)..."
	@mkdir -p $(INSTALL_DIR)
	@cp $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@chmod +x $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installation complete! Run 'batterycap' to check status."

clean:
	@echo "Cleaning artifacts..."
	@rm -f $(BINARY_NAME)

run: build
	./$(BINARY_NAME) status

help:
	@echo "Makefile targets:"
	@echo "  make build    - Compile native binary"
	@echo "  make install  - Install to ~/.local/bin/$(BINARY_NAME)"
	@echo "  make run      - Build and run status"
	@echo "  make clean    - Remove built binary"
