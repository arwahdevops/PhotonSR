#!/bin/sh
set -e # Exit immediately if a command exits with a non-zero status.

# --- Configuration ---
GITHUB_REPO="arwahdevops/PhotonSR"
BINARY_NAME="photonsr"
DEFAULT_INSTALL_DIR="/usr/local/bin" # Default for standard systems
INSTALL_DIR="" # Will be set based on environment

# --- Helper Functions ---
msg() {
  echo "\033[0;32m[PhotonSR Installer]\033[0m $1"
}

err_exit() {
  echo "\033[0;31m[PhotonSR Installer ERROR]\033[0m $1" >&2
  exit 1
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

# --- Environment Detection & Setup ---
setup_environment() {
  # Detect Termux
  if [ -n "$TERMUX_VERSION" ] || [ -d "/data/data/com.termux/files/usr" ]; then
    msg "Termux environment detected."
    INSTALL_DIR="$PREFIX/bin" # $PREFIX is a Termux environment variable
    if [ -z "$INSTALL_DIR" ]; then # Fallback if $PREFIX is not set for some reason
        INSTALL_DIR="/data/data/com.termux/files/usr/bin"
    fi
    # Check if tar is available in Termux (usually is)
    if ! has_command "tar"; then
      err_exit "'tar' command not found. In Termux, you can install it with 'pkg install tar'."
    fi
  else
    INSTALL_DIR="$DEFAULT_INSTALL_DIR"
  fi
  msg "Installation directory set to: $INSTALL_DIR"
}


# --- OS and Architecture Detection ---
get_os_arch() {
  OS_TYPE="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH_TYPE="$(uname -m)"

  case "$OS_TYPE" in
    linux) OS_NAME="linux" ;;
    darwin) OS_NAME="darwin" ;;
    *) err_exit "Unsupported Operating System: $OS_TYPE" ;;
  esac

  # Termux on Android reports arm or aarch64, which is fine.
  # uname -m on Termux for arm64 devices usually shows aarch64.
  case "$ARCH_TYPE" in
    x86_64 | amd64) ARCH_NAME="amd64" ;;
    arm64 | aarch64) ARCH_NAME="arm64" ;; # Map aarch64 to arm64 for asset naming consistency
    armv7l | armhf) ARCH_NAME="arm" ;; # For 32-bit ARM if you support it
    *) err_exit "Unsupported Architecture: $ARCH_TYPE" ;;
  esac

  echo "${OS_NAME}_${ARCH_NAME}"
}

# --- Main Logic ---
main() {
  setup_environment # Set INSTALL_DIR based on environment

  # Check dependencies
  if ! has_command "curl"; then
    err_exit "'curl' command not found. Please install it first."
  fi

  OS_ARCH_COMBO=$(get_os_arch)
  OS_NAME=$(echo "$OS_ARCH_COMBO" | cut -d'_' -f1)
  ARCH_NAME=$(echo "$OS_ARCH_COMBO" | cut -d'_' -f2)

  msg "Detected OS: $OS_NAME, Architecture: $ARCH_NAME"

  LATEST_RELEASE_API_URL="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
  msg "Fetching latest release information from $LATEST_RELEASE_API_URL..."

  # Get the latest tag first, as your assets include version numbers
  TAG=$(curl -sL "$LATEST_RELEASE_API_URL" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
  if [ -z "$TAG" ]; then
      err_exit "Could not fetch the latest release tag."
  fi
  msg "Latest tag: $TAG"

  # Construct the expected asset name with the version tag
  # Your asset name is: photonsr_v0.1.0_linux_arm64.tar.gz
  EXPECTED_ASSET_FILENAME="${BINARY_NAME}_${TAG}_${OS_NAME}_${ARCH_NAME}.tar.gz"

  DOWNLOAD_URL=$(curl -sL "$LATEST_RELEASE_API_URL" | \
    grep "browser_download_url.*${EXPECTED_ASSET_FILENAME}" | \
    cut -d '"' -f 4 | \
    head -n 1)

  if [ -z "$DOWNLOAD_URL" ]; then
      err_exit "Could not find a download URL for asset '$EXPECTED_ASSET_FILENAME'. Please check your GitHub release assets."
  fi

  msg "Download URL: $DOWNLOAD_URL"

  TMP_DIR=$(mktemp -d)
  trap 'rm -rf "$TMP_DIR"' EXIT # Clean up temp dir on exit

  DOWNLOADED_ARCHIVE_PATH="${TMP_DIR}/${EXPECTED_ASSET_FILENAME}"
  EXTRACTED_BINARY_PATH="${TMP_DIR}/${BINARY_NAME}" # Path where the binary will be after extraction

  msg "Downloading $EXPECTED_ASSET_FILENAME to $DOWNLOADED_ARCHIVE_PATH..."
  if ! curl --progress-bar -Lo "$DOWNLOADED_ARCHIVE_PATH" "$DOWNLOAD_URL"; then
    err_exit "Failed to download the archive."
  fi
  msg "Archive downloaded."

  msg "Extracting $BINARY_NAME from $DOWNLOADED_ARCHIVE_PATH..."
  # Assumes the tar.gz contains the binary directly, or in a known path
  # If the binary is inside a folder within the tar.gz, adjust the tar command.
  # Example: if binary is in 'photonsr_v0.1.0_linux_arm64/photonsr'
  # tar -xzf "$DOWNLOADED_ARCHIVE_PATH" -C "$TMP_DIR" --strip-components=1 "${BINARY_NAME}_${TAG}_${OS_NAME}_${ARCH_NAME}/${BINARY_NAME}"
  # For simplicity, assuming the binary is at the root of the tar.gz or the only file.
  if ! tar -xzf "$DOWNLOADED_ARCHIVE_PATH" -C "$TMP_DIR" "${BINARY_NAME}"; then
    # Fallback if the binary name inside tar is different or nested
    # Try to extract the first executable found, or a file named $BINARY_NAME
    # This is more complex and fragile. It's best if your tar.gz structure is consistent.
    msg "Initial extraction failed. Trying to find '${BINARY_NAME}' within the archive..."
    # List contents and try to extract if binary name matches
    tar -tzf "$DOWNLOADED_ARCHIVE_PATH" | grep -qE "/?${BINARY_NAME}$"
    if [ $? -eq 0 ]; then
        # Find the full path of the binary within the tar
        BINARY_IN_TAR=$(tar -tzf "$DOWNLOADED_ARCHIVE_PATH" | grep -E "/?${BINARY_NAME}$" | head -n 1)
        if [ -n "$BINARY_IN_TAR" ]; then
            msg "Found binary as '$BINARY_IN_TAR' in archive. Extracting..."
            if ! tar -xzf "$DOWNLOADED_ARCHIVE_PATH" -C "$TMP_DIR" "$BINARY_IN_TAR"; then
                 err_exit "Failed to extract '$BINARY_IN_TAR' from the archive."
            fi
            # If extracted into a subdirectory, move it to $EXTRACTED_BINARY_PATH
            if [ ! -f "$EXTRACTED_BINARY_PATH" ] && [ -f "${TMP_DIR}/${BINARY_IN_TAR}" ]; then
                mv "${TMP_DIR}/${BINARY_IN_TAR}" "$EXTRACTED_BINARY_PATH"
            fi
        else
            err_exit "Could not find '$BINARY_NAME' within the archive after initial extraction attempt."
        fi
    else
      err_exit "Failed to extract '$BINARY_NAME' from the archive. Ensure the tar.gz contains a file named '$BINARY_NAME'."
    fi
  fi


  if [ ! -f "$EXTRACTED_BINARY_PATH" ]; then
    err_exit "Binary '$BINARY_NAME' not found after extraction at $EXTRACTED_BINARY_PATH."
  fi
  chmod +x "$EXTRACTED_BINARY_PATH"
  msg "Binary extracted and made executable."

  # Installation
  SUDO_CMD=""
  # Only check for sudo if not in Termux and trying to write to a protected directory
  if [ -z "$TERMUX_VERSION" ] && [ "$(id -u)" -ne 0 ] && ! [ -w "$INSTALL_DIR" ]; then
    msg "Sudo privileges are required to install to $INSTALL_DIR"
    if has_command "sudo"; then
      SUDO_CMD="sudo"
    else
      err_exit "'sudo' command not found. Please run this script as root or ensure you have write permissions to $INSTALL_DIR."
    fi
  fi

  DEST_PATH="${INSTALL_DIR}/${BINARY_NAME}"
  msg "Installing $BINARY_NAME to $DEST_PATH..."
  if ! ${SUDO_CMD} mv "$EXTRACTED_BINARY_PATH" "$DEST_PATH"; then
    err_exit "Failed to install the binary to $DEST_PATH. (Command: ${SUDO_CMD} mv \"$EXTRACTED_BINARY_PATH\" \"$DEST_PATH\")"
  fi

  msg ""
  msg "PhotonSR was successfully installed to $DEST_PATH"
  if has_command "$BINARY_NAME"; then
    msg "You can now run '$BINARY_NAME'. Try '$BINARY_NAME --version'."
  else
    msg "Please open a new terminal or run 'source ~/.bashrc' (or your shell's equivalent config file) for the command to be available."
    msg "Then, try running: $BINARY_NAME --version"
  fi
}

# Run the main function
main
