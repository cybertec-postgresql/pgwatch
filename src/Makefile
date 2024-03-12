# Makefile

# The name of the Go binary to build
BINARY_NAME=pwctl

# The directory to install the binary to
INSTALL_DIR=/usr/local/bin

# The Go command
GO=go

# The default target of this Makefile
all: permissions

# Compiles the Go program
build:
	$(GO) build -o $(BINARY_NAME)

# Installs the Go program as a command-line tool
install: build
	mv $(BINARY_NAME) $(INSTALL_DIR)

# Provide necessary permissions to the binary
permissions: install
	chmod +x $(INSTALL_DIR)/$(BINARY_NAME)

# Cleans up the binary
clean:
	if [ -f $(INSTALL_DIR)/$(BINARY_NAME) ] ; then rm $(INSTALL_DIR)/$(BINARY_NAME) ; fi