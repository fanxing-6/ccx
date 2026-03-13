#!/usr/bin/env bash
set -euo pipefail

ccx_repo="${CCX_REPO:-fanxing-6/ccx}"
ccx_binary_name="${CCX_BINARY_NAME:-ccx}"
ccx_asset_name="${CCX_ASSET_NAME:-ccx_linux_amd64.tar.gz}"
ccx_install_dir="${CCX_INSTALL_DIR:-/usr/local/bin}"
ccx_version="${CCX_VERSION:-latest}"
ccx_release_base_url="${CCX_RELEASE_BASE_URL:-https://github.com/${ccx_repo}/releases}"
ccx_download_url="${CCX_DOWNLOAD_URL:-}"

say() {
  printf '[ccx-install] %s\n' "$*"
}

die() {
  printf '[ccx-install] %s\n' "$*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "缺少依赖命令: $1"
}

ensure_platform() {
  local os arch
  os=$(uname -s)
  arch=$(uname -m)

  [ "$os" = "Linux" ] || die "当前仅支持 Linux，检测到: $os"

  case "$arch" in
    x86_64|amd64)
      ;;
    *)
      die "当前仅支持 linux/amd64，检测到: $os/$arch"
      ;;
  esac
}

build_download_url() {
  if [ -n "$ccx_download_url" ]; then
    printf '%s\n' "$ccx_download_url"
    return
  fi

  if [ "$ccx_version" = "latest" ]; then
    printf '%s/latest/download/%s\n' "$ccx_release_base_url" "$ccx_asset_name"
    return
  fi

  case "$ccx_version" in
    v*)
      printf '%s/download/%s/%s\n' "$ccx_release_base_url" "$ccx_version" "$ccx_asset_name"
      ;;
    *)
      printf '%s/download/v%s/%s\n' "$ccx_release_base_url" "$ccx_version" "$ccx_asset_name"
      ;;
  esac
}

download_file() {
  local url="$1"
  local dest="$2"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
    return
  fi

  if command -v wget >/dev/null 2>&1; then
    wget -qO "$dest" "$url"
    return
  fi

  die "未找到 curl 或 wget，无法下载安装包"
}

ensure_install_dir() {
  if mkdir -p "$ccx_install_dir" 2>/dev/null; then
    return
  fi

  command -v sudo >/dev/null 2>&1 || die "无法写入安装目录且未找到 sudo: $ccx_install_dir"
  sudo mkdir -p "$ccx_install_dir"
}

install_binary() {
  local source_path="$1"
  local target_path="$ccx_install_dir/$ccx_binary_name"

  if install -m 0755 "$source_path" "$target_path" 2>/dev/null; then
    return
  fi

  command -v sudo >/dev/null 2>&1 || die "无法写入安装目录且未找到 sudo: $ccx_install_dir"
  sudo install -m 0755 "$source_path" "$target_path"
}

main() {
  local tmp_dir archive_path extracted_path download_url version_output

  ensure_platform
  require_cmd tar

  download_url=$(build_download_url)
  tmp_dir=$(mktemp -d)
  archive_path="$tmp_dir/$ccx_asset_name"
  extracted_path="$tmp_dir/$ccx_binary_name"
  trap "rm -rf -- '$tmp_dir'" EXIT

  say "下载安装包: $download_url"
  download_file "$download_url" "$archive_path"

  say "解压安装包"
  tar -xzf "$archive_path" -C "$tmp_dir" "$ccx_binary_name"
  [ -f "$extracted_path" ] || die "安装包中缺少 $ccx_binary_name"
  chmod +x "$extracted_path"

  say "安装到: $ccx_install_dir"
  ensure_install_dir
  install_binary "$extracted_path"

  version_output=$("$ccx_install_dir/$ccx_binary_name" --version 2>/dev/null || true)
  if [ -n "$version_output" ]; then
    say "安装完成: $version_output"
  else
    say "安装完成: $ccx_install_dir/$ccx_binary_name"
  fi

  case ":$PATH:" in
    *":$ccx_install_dir:"*)
      ;;
    *)
      say "提示: 请确认 $ccx_install_dir 已加入 PATH"
      ;;
  esac

  say "下一步可运行: ccx init"
}

main "$@"
