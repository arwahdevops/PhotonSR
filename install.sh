#!/bin/bash

# Skrip untuk menginstal atau memperbarui rilis terbaru PhotonSR dari GitHub,
# dengan dukungan untuk mengunduh dan mengekstrak arsip.

set -euf -o pipefail
# set -x # Uncomment untuk debugging mendalam

# --- Konfigurasi Proyek ---
PROJECT_OWNER="arwahdevops"
PROJECT_REPO="PhotonSR"
TARGET_BIN_NAME="photonsr"

# --- Variabel Global ---
OS_TYPE=""
ARCH_TYPE=""
# ASSET_PATTERN_TO_SEARCH="" # Tidak begitu krusial lagi
INSTALL_DIR=""
DOWNLOAD_URL=""
RELEASE_TAG=""
GLOBAL_TMP_DIR=""

# --- Fungsi Utilitas ---
info() { echo "[INFO] $1"; }
warn() { echo "[WARN] $1" >&2; }
error_exit() {
    echo "[ERROR] $1" >&2
    exit 1 # Trap EXIT akan menangani pembersihan
}

check_command() {
    if ! command -v "$1" &> /dev/null; then
        error_exit "'$1' tidak ditemukan. Harap instal '$1' untuk melanjutkan. $2"
    fi
}

cleanup_on_exit() {
    local exit_status=$?
    if [ -n "$GLOBAL_TMP_DIR" ] && [ -d "$GLOBAL_TMP_DIR" ]; then
        info "Membersihkan direktori sementara global: ${GLOBAL_TMP_DIR} (status keluar: $exit_status)"
        rm -rf -- "$GLOBAL_TMP_DIR"
    elif [ "$exit_status" -eq 0 ]; then
        info "Tidak ada direktori sementara global utama untuk dibersihkan atau sudah dibersihkan."
    fi
}
trap cleanup_on_exit EXIT SIGINT SIGTERM

determine_os_arch_asset_pattern() {
    info "Mendeteksi OS dan arsitektur..."
    local os_kernel=$(uname -s)
    case "${os_kernel}" in
        Linux*)  OS_TYPE="linux";;
        Darwin*) OS_TYPE="darwin";;
        MINGW*|MSYS*|CYGWIN*) OS_TYPE="windows";;
        *) error_exit "OS tidak didukung: ${os_kernel}";;
    esac

    local arch_raw=$(uname -m)
    case "${arch_raw}" in
        x86_64|amd64)         ARCH_TYPE="amd64";;
        armv8*|aarch64*|arm64) ARCH_TYPE="arm64";;
        armv7*|arm)           ARCH_TYPE="arm";;
        i386|i686)            ARCH_TYPE="386";;
        *) error_exit "Arsitektur tidak didukung: ${arch_raw}";;
    esac

    if [ -n "$TERMUX_VERSION" ]; then
        info "Termux terdeteksi. Menggunakan direktori instalasi $PREFIX/bin."
        INSTALL_DIR="$PREFIX/bin"
    elif [ "$OS_TYPE" == "windows" ]; then
        if [ -n "$ProgramFiles" ]; then INSTALL_DIR="${ProgramFiles}/${PROJECT_REPO}";
        elif [ -n "$ProgramFiles_x86" ]; then INSTALL_DIR="${ProgramFiles_x86}/${PROJECT_REPO}";
        else INSTALL_DIR="./${PROJECT_REPO}_installed"; warn "Variabel ProgramFiles tidak ditemukan. Instalasi akan dilakukan di ${INSTALL_DIR}"; fi
        info "Windows terdeteksi. Direktori instalasi akan dicoba di: ${INSTALL_DIR}"
    else
        INSTALL_DIR="/usr/local/bin"
    fi

    info "OS: ${OS_TYPE}, Arsitektur: ${ARCH_TYPE}"
    info "Direktori Instalasi Target: ${INSTALL_DIR}"
}

fetch_latest_release_url() {
    info "Mengambil informasi rilis terbaru..."
    check_command "curl" "Petunjuk: sudo apt install curl / brew install curl / pkg install curl"
    check_command "jq" "Petunjuk: sudo apt install jq / brew install jq / pkg install jq"

    local api_url="https://api.github.com/repos/${PROJECT_OWNER}/${PROJECT_REPO}/releases/latest"
    local response_json

    response_json=$(
        local temp_response_file
        local http_status_sub # Variabel status HTTP khusus untuk subshell
        temp_response_file=$(mktemp -t "${TARGET_BIN_NAME}_api_body.XXXXXX")
        if [ -z "$temp_response_file" ] || [ ! -f "$temp_response_file" ] ; then
             # Menggunakan echo ke stderr dan exit 1 agar subshell gagal
             echo "[ERROR_SUB] Gagal membuat file sementara untuk respons API." >&2
             exit 1 # Ini akan menghentikan subshell dan $? akan non-nol
        fi

        trap 'rm -f "$temp_response_file" >/dev/null 2>&1; unset temp_response_file;' RETURN EXIT SIGINT SIGTERM

        http_status_sub=$(curl -sL -w "%{http_code}" "${api_url}" -o "$temp_response_file")

        if [ "$?" -ne 0 ]; then
            echo "[ERROR_SUB] curl gagal menghubungi API GitHub." >&2
            exit 1
        fi
        if [ "$http_status_sub" != "200" ]; then
            local err_content_sub=$(cat "$temp_response_file" 2>/dev/null)
            echo "[ERROR_SUB] Gagal mengambil info rilis dari API. Status: ${http_status_sub}. Respons: ${err_content_sub}" >&2
            exit 1
        fi
        cat "$temp_response_file"
    )

    # Periksa status keluar dari subshell. Jika non-nol, berarti ada error di dalam subshell.
    # `set -e` akan menangani ini jika subshell keluar dengan status non-nol.
    # Pengecekan tambahan jika $response_json kosong untuk keamanan.
    if [ -z "$response_json" ]; then
        error_exit "Gagal mendapatkan respons JSON dari API call (subshell mungkin gagal atau menghasilkan output kosong).";
    fi

    RELEASE_TAG=$(echo "$response_json" | jq -r ".tag_name")
    if [ -z "$RELEASE_TAG" ] || [ "$RELEASE_TAG" == "null" ]; then error_exit "Tidak dapat menemukan tag rilis terbaru."; fi
    info "Rilis terbaru ditemukan: ${RELEASE_TAG}"

    local full_asset_pattern_base="${PROJECT_REPO,,}_${RELEASE_TAG}_${OS_TYPE}_${ARCH_TYPE}"
    local asset_ext
    if [ "$OS_TYPE" == "windows" ]; then asset_ext=".zip"; else asset_ext=".tar.gz"; fi
    local full_asset_name_expected="${full_asset_pattern_base}${asset_ext}"
    info "Nama aset arsip yang diharapkan: ${full_asset_name_expected}"

    DOWNLOAD_URL=$(echo "$response_json" | jq -r ".assets[] | select(.name == \"${full_asset_name_expected}\") | .browser_download_url" | head -n 1)

    if [ -z "$DOWNLOAD_URL" ]; then
        error_exit "Tidak dapat menemukan URL download untuk aset '${full_asset_name_expected}'. Aset yang tersedia di rilis ${RELEASE_TAG}:\n$(echo "$response_json" | jq -r '.assets[].name')"
    fi
    info "URL Download Arsip Ditemukan: ${DOWNLOAD_URL}"
}

install_binary_from_archive() {
    info "Memulai proses download dan instalasi dari arsip..."
    GLOBAL_TMP_DIR=$(mktemp -d -t "${TARGET_BIN_NAME}_install.XXXXXX")
    info "Direktori sementara utama dibuat: ${GLOBAL_TMP_DIR}"

    local downloaded_archive_name=$(basename "${DOWNLOAD_URL}")
    local downloaded_archive_path="${GLOBAL_TMP_DIR}/${downloaded_archive_name}"
    local extract_dir="${GLOBAL_TMP_DIR}/extracted"

    info "Mengunduh arsip ${downloaded_archive_name} dari ${DOWNLOAD_URL}..."
    if ! curl --progress-bar -fL "${DOWNLOAD_URL}" -o "${downloaded_archive_path}"; then
        error_exit "Download arsip gagal dari ${DOWNLOAD_URL}."
    fi
    if [ ! -s "$downloaded_archive_path" ]; then error_exit "Arsip yang diunduh kosong atau tidak valid."; fi
    info "Download arsip selesai: ${downloaded_archive_path}"

    info "Mengekstrak arsip..."
    mkdir -p "$extract_dir"
    local extracted_binary_path=""

    if [[ "${downloaded_archive_name}" == *.tar.gz ]]; then
        check_command "tar" "Petunjuk: Biasanya sudah terinstal."
        if ! tar -xzf "$downloaded_archive_path" -C "$extract_dir"; then
            error_exit "Gagal mengekstrak ${downloaded_archive_name}"
        fi
    elif [[ "${downloaded_archive_name}" == *.zip ]]; then
        check_command "unzip" "Petunjuk: sudo apt install unzip / pkg install unzip"
        if ! unzip -q "$downloaded_archive_path" -d "$extract_dir"; then
            error_exit "Gagal mengekstrak ${downloaded_archive_name}"
        fi
    else
        error_exit "Format arsip tidak dikenal atau tidak didukung: ${downloaded_archive_name}"
    fi
    info "Ekstraksi selesai ke ${extract_dir}"

    local binary_to_find="$TARGET_BIN_NAME"
    if [ "$OS_TYPE" == "windows" ]; then binary_to_find="${TARGET_BIN_NAME}.exe"; fi

    if [ -f "${extract_dir}/${binary_to_find}" ]; then
        extracted_binary_path="${extract_dir}/${binary_to_find}"
    else
        warn "Biner tidak ditemukan di root ekstraksi, mencari di subdirektori..."
        extracted_binary_path=$(find "$extract_dir" -mindepth 1 -maxdepth 2 -type f -name "$binary_to_find" -print -quit)
        if [ -z "$extracted_binary_path" ]; then
            warn "Mencoba mencari file executable bernama '${TARGET_BIN_NAME}' (tanpa .exe untuk Windows)..."
            extracted_binary_path=$(find "$extract_dir" -mindepth 1 -maxdepth 2 -type f -name "$TARGET_BIN_NAME" -executable -print -quit)
        fi
    fi

    if [ -z "$extracted_binary_path" ] || [ ! -f "$extracted_binary_path" ]; then
        error_exit "Tidak dapat menemukan biner '${binary_to_find}' (atau '${TARGET_BIN_NAME}') di dalam arsip yang diekstrak. Isi direktori ekstraksi (${extract_dir}):\n$(ls -R "$extract_dir")"
    fi
    info "Biner ditemukan setelah ekstraksi: ${extracted_binary_path}"

    chmod +x "${extracted_binary_path}"

    local full_install_path="${INSTALL_DIR}/${TARGET_BIN_NAME}"
    if [ "$OS_TYPE" == "windows" ] && [[ ! "$full_install_path" == *.exe ]]; then
        full_install_path="${full_install_path}.exe"
    fi

    info "Mempersiapkan instalasi biner ke ${full_install_path}..."
    if [ ! -d "$INSTALL_DIR" ]; then
        info "Direktori instalasi ${INSTALL_DIR} tidak ada, mencoba membuat..."
        if [ "$(id -u)" = "0" ] || { [ -d "$(dirname "$INSTALL_DIR")" ] && [ -w "$(dirname "$INSTALL_DIR")" ]; } || [ -w "$INSTALL_DIR" ]; then
            if ! mkdir -p "$INSTALL_DIR"; then error_exit "Gagal membuat direktori ${INSTALL_DIR}."; fi
        elif command -v sudo &> /dev/null; then
            info "Memerlukan hak akses sudo untuk membuat direktori ${INSTALL_DIR}."
            if ! sudo mkdir -p "$INSTALL_DIR"; then error_exit "Gagal membuat direktori ${INSTALL_DIR} dengan sudo."; fi
        else
            error_exit "Tidak dapat membuat direktori instalasi ${INSTALL_DIR}."
        fi
        info "Direktori ${INSTALL_DIR} berhasil dibuat/diverifikasi."
    fi

    info "Menginstal biner..."
    if [ "$(id -u)" = "0" ] || [ -w "$INSTALL_DIR" ]; then
        if ! install -v -m 755 "$extracted_binary_path" "$full_install_path"; then
            error_exit "Gagal menginstal biner ke ${full_install_path}."
        fi
    elif command -v sudo &> /dev/null; then
        info "Memerlukan hak akses sudo untuk menginstal ke ${INSTALL_DIR}."
        if ! sudo install -v -m 755 "$extracted_binary_path" "$full_install_path"; then
            error_exit "Gagal menginstal biner ke ${full_install_path} dengan sudo."
        fi
    else
        warn "Tidak dapat menulis ke direktori instalasi ${INSTALL_DIR}."
        warn "Biner yang telah diekstrak dan siap pakai ada di: ${extracted_binary_path}"
        warn "Anda bisa menyalinnya secara manual."
        trap - EXIT SIGINT SIGTERM
        GLOBAL_TMP_DIR=""
        exit 1
    fi
    info "${TARGET_BIN_NAME} berhasil diinstal ke ${full_install_path}"

    local final_bin_to_check="$TARGET_BIN_NAME"
    if [ "$OS_TYPE" == "windows" ] && [[ ! "$final_bin_to_check" == *.exe ]]; then
        final_bin_to_check="${final_bin_to_check}.exe"
    fi

    if command -v "$final_bin_to_check" &> /dev/null; then
        info "Validasi: '${final_bin_to_check}' ditemukan di PATH."
        echo -n "[INFO] Versi terinstal: "
        "$final_bin_to_check" -version
    elif [ -x "$full_install_path" ]; then
        warn "Validasi: '${final_bin_to_check}' terinstal di ${full_install_path}, tetapi ${INSTALL_DIR} mungkin belum ada di PATH Anda."
        warn "Untuk menjalankan: ${full_install_path} -version"
        echo -n "[INFO] Versi terinstal (via path lengkap): "
        "$full_install_path" -version
    else
        error_exit "Instalasi tampaknya gagal. Biner tidak ditemukan atau tidak dapat dieksekusi di ${full_install_path}."
    fi
}

main() {
    info "Memulai skrip instalasi/pembaruan untuk ${TARGET_BIN_NAME} (dari ${PROJECT_OWNER}/${PROJECT_REPO})"

    determine_os_arch_asset_pattern
    fetch_latest_release_url
    install_binary_from_archive

    info ""
    info "${TARGET_BIN_NAME} telah berhasil diinstal/diperbarui."
    info "Jika ${INSTALL_DIR} baru saja ditambahkan ke PATH Anda, mungkin perlu memulai ulang shell."

    # Pembersihan GLOBAL_TMP_DIR secara eksplisit di sini tidak lagi diperlukan
    # karena trap EXIT akan menanganinya.
    # Jika ada error sebelum titik ini, trap EXIT juga akan membersihkannya.
}

if [ "${BASH_SOURCE[0]}" -ef "$0" ]; then
    main "$@"
fi
