#!/usr/bin/env bash
# render-overlays.sh — 渲染 kustomize 产物，按区域替换 __*__ 占位符。
#
# 企业级理由 (ADR-014/016)：三区域 overlay + global 层使用 delimited __*__
# 占位避免与正常字段冲突。CI/CD 必须在 deploy / dry-run 前把所有占位替换为
# 环境注入值，否则产物含残留占位会导致 K8s apply 静默失败——资源字段值变成
# 字面量 "__REDIS_HOST_ASIA_SOUTHEAST1__" 而非真实主机名，Pod 启动时才崩。
#
# Usage:
#   kubectl kustomize infra/k8s/overlays/<region> | bash scripts/ci/render-overlays.sh <region>
#
# 从 stdin 读取 kustomize YAML，向 stdout 写入渲染后的 YAML。
# 幂等：对已渲染产物再跑一次是 no-op（sed 无匹配则原样输出）。
#
# 必需环境变量（见 usage() 详情）：
#   GCP_PROJECT_ID, IMAGE_TAG, TRUSTED_PROXY_CIDRS,
#   GLOBAL_STATIC_IP_NAME, GLOBAL_CERT_NAME, INGRESS_NAMESPACE,
#   REDIS_HOST_<REGION>, DB_HOST_<REGION>
#
# <REGION> 后缀是区域名的 SCREAMING_SNAKE_CASE，例如 asia-southeast1 对应
# REDIS_HOST_ASIA_SOUTHEAST1 / DB_HOST_ASIA_SOUTHEAST1。
#
# 占位清单（8 类，由 A1 维护，本脚本只替换不修改源 yaml）：
#   通用（每区域 overlay）：
#     __GCP_PROJECT_ID__          — image ref + server/worker SA 注解
#     __IMAGE_TAG__               — image tag
#     __TRUSTED_PROXY_CIDRS__     — ConfigMap trusted-proxy-cidrs
#   区域特定（仅匹配区域的 overlay）：
#     __REDIS_HOST_<REGION>__     — ConfigMap redis-host
#     __DB_HOST_<REGION>__        — ConfigMap db-host
#   全局（global/ 层经 ../../global 引入 overlay 产物）：
#     __GLOBAL_STATIC_IP_NAME__   — MultiClusterIngress annotation
#     __GLOBAL_CERT_NAME__        — MultiClusterIngress annotation
#     __INGRESS_NAMESPACE__       — NetworkPolicy namespaceSelector

set -euo pipefail

usage() {
  cat <<EOF
Usage: kubectl kustomize infra/k8s/overlays/<region> | $0 <region>

Replaces __*__ placeholders in kustomize output for the given region.
Region must be one of: asia-southeast1, europe-west1, us-east1.

Required environment variables:
  GCP_PROJECT_ID            GCP project hosting GCR / GKE
  IMAGE_TAG                 Immutable image tag (typically git SHA)
  TRUSTED_PROXY_CIDRS       Comma-separated trusted proxy CIDRs
  GLOBAL_STATIC_IP_NAME     Global Anycast static IP name (MCI)
  GLOBAL_CERT_NAME          Google-managed cert name (MCI)
  INGRESS_NAMESPACE         Ingress controller namespace (gke-managed-system | kube-system)
  REDIS_HOST_<REGION>       Region-specific Memorystore Redis host
  DB_HOST_<REGION>          Region-specific Cloud SQL DB host

The <REGION> suffix is the region in SCREAMING_SNAKE_CASE, e.g.
REDIS_HOST_ASIA_SOUTHEAST1 for asia-southeast1.

Exit codes:
  0  success (all placeholders substituted, output on stdout)
  1  required env var missing
  2  invalid usage (wrong args / unsupported region)
EOF
}

if [[ $# -ne 1 ]]; then
  usage
  echo "ERROR: region argument required (got $# args)" >&2
  exit 2
fi

REGION="$1"
case "$REGION" in
  asia-southeast1|europe-west1|us-east1) ;;
  *)
    usage
    echo "ERROR: unsupported region: $REGION (supported: asia-southeast1, europe-west1, us-east1)" >&2
    exit 2
    ;;
esac

# region → SCREAMING_SNAKE_CASE (asia-southeast1 → ASIA_SOUTHEAST1)
REGION_UPPER="$(printf '%s' "${REGION//-/_}" | tr '[:lower:]' '[:upper:]')"

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "ERROR: required env var $name is not set (region=$REGION)" >&2
    exit 1
  fi
}

# 通用占位（每区域 overlay 都有）
require_env GCP_PROJECT_ID
require_env IMAGE_TAG
require_env TRUSTED_PROXY_CIDRS
# 全局占位（global/ 层经 overlay 引入）
require_env GLOBAL_STATIC_IP_NAME
require_env GLOBAL_CERT_NAME
require_env INGRESS_NAMESPACE
# 区域特定占位（仅当前区域的 REDIS/DB host）
REDIS_ENV="REDIS_HOST_${REGION_UPPER}"
DB_ENV="DB_HOST_${REGION_UPPER}"
require_env "$REDIS_ENV"
require_env "$DB_ENV"

REDIS_HOST="${!REDIS_ENV}"
DB_HOST="${!DB_ENV}"

# 单次 sed 调用 + 多 -e 表达式：幂等且高效。
# 用 | 作 sed 分隔符，容忍 TRUSTED_PROXY_CIDRS 中的逗号和斜杠。
# 顺序无关（各占位互不重叠），但区域特定占位放一起便于阅读。
sed \
  -e "s|__GCP_PROJECT_ID__|${GCP_PROJECT_ID}|g" \
  -e "s|__IMAGE_TAG__|${IMAGE_TAG}|g" \
  -e "s|__TRUSTED_PROXY_CIDRS__|${TRUSTED_PROXY_CIDRS}|g" \
  -e "s|__REDIS_HOST_${REGION_UPPER}__|${REDIS_HOST}|g" \
  -e "s|__DB_HOST_${REGION_UPPER}__|${DB_HOST}|g" \
  -e "s|__GLOBAL_STATIC_IP_NAME__|${GLOBAL_STATIC_IP_NAME}|g" \
  -e "s|__GLOBAL_CERT_NAME__|${GLOBAL_CERT_NAME}|g" \
  -e "s|__INGRESS_NAMESPACE__|${INGRESS_NAMESPACE}|g"
