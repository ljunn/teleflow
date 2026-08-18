#!/usr/bin/env bash
set -euo pipefail

REPOSITORY="${TELEFLOW_GITHUB_REPOSITORY:-ljunn/teleflow}"
INSTALL_DIR="/opt/teleflow"
DATA_DIR="/var/lib/teleflow"
CONFIG_DIR="/etc/teleflow"

if [[ "${EUID}" -ne 0 ]]; then
  echo "请使用 sudo 运行安装脚本。" >&2
  exit 1
fi

for command in curl tar sha256sum; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    echo "缺少依赖：${command}" >&2
    exit 1
  fi
done

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "暂不支持当前架构：$(uname -m)" >&2; exit 1 ;;
esac

VERSION="${TELEFLOW_VERSION:-}"
if [[ -z "${VERSION}" ]]; then
  VERSION="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${REPOSITORY}/releases/latest" | awk -F/ '{print $NF}')"
fi
if [[ ! "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "无法解析发布版本：${VERSION}" >&2
  exit 1
fi

VERSION_NUMBER="${VERSION#v}"
ARCHIVE="teleflow_${VERSION_NUMBER}_linux_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPOSITORY}/releases/download/${VERSION}"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TEMP_DIR}"' EXIT

echo "正在下载 Teleflow ${VERSION} (${ARCH})..."
curl -fL --retry 3 "${BASE_URL}/${ARCHIVE}" -o "${TEMP_DIR}/${ARCHIVE}"
curl -fL --retry 3 "${BASE_URL}/checksums.txt" -o "${TEMP_DIR}/checksums.txt"

EXPECTED="$(awk -v file="${ARCHIVE}" '$2 == file {print $1}' "${TEMP_DIR}/checksums.txt")"
if [[ -z "${EXPECTED}" ]]; then
  echo "校验文件中找不到 ${ARCHIVE}" >&2
  exit 1
fi
echo "${EXPECTED}  ${TEMP_DIR}/${ARCHIVE}" | sha256sum --check --status

tar -xzf "${TEMP_DIR}/${ARCHIVE}" -C "${TEMP_DIR}" teleflow teleflow.service
id teleflow >/dev/null 2>&1 || useradd --system --home-dir "${DATA_DIR}" --shell /usr/sbin/nologin teleflow
install -d -o teleflow -g teleflow -m 0755 "${INSTALL_DIR}"
install -d -m 0755 "${CONFIG_DIR}"
install -d -o teleflow -g teleflow -m 0750 "${DATA_DIR}"
install -o teleflow -g teleflow -m 0755 "${TEMP_DIR}/teleflow" "${INSTALL_DIR}/teleflow"
install -m 0644 "${TEMP_DIR}/teleflow.service" /etc/systemd/system/teleflow.service

if [[ ! -f "${CONFIG_DIR}/teleflow.env" ]]; then
  cat >"${CONFIG_DIR}/teleflow.env" <<EOF
TELEFLOW_ADDR=:8080
TELEFLOW_DATA_DIR=${DATA_DIR}
TELEFLOW_PUBLIC_URL=http://localhost:8080
TELEFLOW_GITHUB_REPOSITORY=${REPOSITORY}
TELEFLOW_TELEGRAM_API_ID=
TELEFLOW_TELEGRAM_API_HASH=
TELEFLOW_RELAY_BOT_TOKEN=
EOF
  chmod 0640 "${CONFIG_DIR}/teleflow.env"
fi

systemctl daemon-reload
systemctl enable teleflow
systemctl restart teleflow

echo "Teleflow ${VERSION} 已安装。"
echo "访问地址：http://服务器IP:8080"
