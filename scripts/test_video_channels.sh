#!/usr/bin/env bash
set -euo pipefail

NEWAPI_URL="${NEWAPI_URL:-http://localhost:3000}"
NEWAPI_KEY="${NEWAPI_KEY:-}"
DOUBAO_CHANNEL_ID="${DOUBAO_CHANNEL_ID:-}"
CHENGMENG_CHANNEL_ID="${CHENGMENG_CHANNEL_ID:-}"
POLL_INTERVAL="${POLL_INTERVAL:-5}"
POLL_MAX_TIMES="${POLL_MAX_TIMES:-30}"

if [[ -z "${NEWAPI_KEY}" ]]; then
  printf '错误：请先设置 NEWAPI_KEY，例如：export NEWAPI_KEY="sk-xxxx"\n' >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  printf '错误：未找到 curl，请先安装 curl。\n' >&2
  exit 1
fi

HAS_PYTHON3=0
if command -v python3 >/dev/null 2>&1; then
  HAS_PYTHON3=1
fi

NEWAPI_URL="${NEWAPI_URL%/}"

printf '============================================================\n'
printf '警告：该脚本会真实调用视频生成接口，可能产生上游计费。\n'
printf '请先确认：渠道已启用、额度足够、模型映射正确、素材链接可访问。\n'
printf '建议先在测试环境或低成本参数下执行。\n'
printf '============================================================\n\n'

build_auth_header() {
  local channel_id="${1:-}"
  local key="${NEWAPI_KEY}"

  if [[ "${key}" == Bearer\ * ]]; then
    key="${key#Bearer }"
  elif [[ "${key}" == bearer\ * ]]; then
    key="${key#bearer }"
  fi

  if [[ -n "${channel_id}" ]]; then
    printf 'Bearer %s-%s' "${key}" "${channel_id}"
  else
    printf 'Bearer %s' "${key}"
  fi
}

pretty_print_json() {
  local body="${1:-}"
  if [[ "${HAS_PYTHON3}" -eq 1 ]]; then
    if BODY="${body}" python3 - <<'PY' >/dev/null 2>&1
import json
import os
json.loads(os.environ["BODY"])
PY
    then
      BODY="${body}" python3 - <<'PY'
import json
import os
print(json.dumps(json.loads(os.environ["BODY"]), ensure_ascii=False, indent=2))
PY
      return 0
    fi
  fi

  printf '%s\n' "${body}"
}

json_extract() {
  local body="${1:-}"
  local candidates="${2:-}"

  if [[ -z "${body}" || -z "${candidates}" ]]; then
    return 1
  fi

  if [[ "${HAS_PYTHON3}" -eq 1 ]]; then
    BODY="${body}" CANDIDATES="${candidates}" python3 - <<'PY'
import json
import os
import sys

body = os.environ.get("BODY", "")
candidates = [item.strip() for item in os.environ.get("CANDIDATES", "").split(",") if item.strip()]

try:
    data = json.loads(body)
except Exception:
    sys.exit(1)

def is_valid(value):
    return value not in (None, "", [], {})

def get_by_path(obj, path):
    current = obj
    for part in path.split("."):
        if isinstance(current, dict) and part in current:
            current = current[part]
        else:
            return None
    return current

for candidate in candidates:
    value = get_by_path(data, candidate)
    if is_valid(value):
        if isinstance(value, (dict, list)):
            print(json.dumps(value, ensure_ascii=False))
        else:
            print(value)
        sys.exit(0)

target_keys = [item.split(".")[-1] for item in candidates]

def walk(obj):
    if isinstance(obj, dict):
        for key in target_keys:
            if key in obj and is_valid(obj[key]):
                return obj[key]
        for value in obj.values():
            found = walk(value)
            if is_valid(found):
                return found
    elif isinstance(obj, list):
        for item in obj:
            found = walk(item)
            if is_valid(found):
                return found
    return None

value = walk(data)
if is_valid(value):
    if isinstance(value, (dict, list)):
        print(json.dumps(value, ensure_ascii=False))
    else:
        print(value)
    sys.exit(0)

sys.exit(1)
PY
    return $?
  fi

  local candidate key result
  IFS=',' read -r -a _candidate_array <<<"${candidates}"
  for candidate in "${_candidate_array[@]}"; do
    key="${candidate##*.}"
    result="$(printf '%s' "${body}" | sed -nE 's/.*"'${key}'"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p' | head -n 1)"
    if [[ -n "${result}" ]]; then
      printf '%s\n' "${result}"
      return 0
    fi

    result="$(printf '%s' "${body}" | sed -nE 's/.*"'${key}'"[[:space:]]*:[[:space:]]*([0-9]+).*/\1/p' | head -n 1)"
    if [[ -n "${result}" ]]; then
      printf '%s\n' "${result}"
      return 0
    fi
  done

  return 1
}

request_json() {
  local method="${1}"
  local url="${2}"
  local auth_header="${3}"
  local body="${4:-}"

  local response_file
  response_file="$(mktemp)"

  local http_code
  if [[ "${method}" == "POST" ]]; then
    if ! http_code="$(
      curl -sS \
        -o "${response_file}" \
        -w "%{http_code}" \
        -X POST "${url}" \
        -H "Authorization: ${auth_header}" \
        -H "Accept: application/json" \
        -H "Content-Type: application/json" \
        --data "${body}"
    )"; then
      rm -f "${response_file}"
      printf '请求失败：POST %s\n' "${url}" >&2
      return 1
    fi
  else
    if ! http_code="$(
      curl -sS \
        -o "${response_file}" \
        -w "%{http_code}" \
        -X GET "${url}" \
        -H "Authorization: ${auth_header}" \
        -H "Accept: application/json"
    )"; then
      rm -f "${response_file}"
      printf '请求失败：GET %s\n' "${url}" >&2
      return 1
    fi
  fi

  RESPONSE_STATUS="${http_code}"
  RESPONSE_BODY="$(<"${response_file}")"
  rm -f "${response_file}"
}

poll_task_if_possible() {
  local name="${1}"
  local auth_header="${2}"
  local task_id="${3}"

  if [[ -z "${task_id}" ]]; then
    printf '[%s] 响应里没有可用 task_id，跳过轮询。\n\n' "${name}"
    return 0
  fi

  printf '[%s] 检测到任务 ID：%s\n' "${name}" "${task_id}"

  local poll_url="${NEWAPI_URL}/v1/video/generations/${task_id}"
  local i state normalized_state

  for ((i = 1; i <= POLL_MAX_TIMES; i++)); do
    request_json "GET" "${poll_url}" "${auth_header}"

    printf '[%s] 第 %d/%d 次轮询，HTTP %s\n' "${name}" "${i}" "${POLL_MAX_TIMES}" "${RESPONSE_STATUS}"

    state="$(json_extract "${RESPONSE_BODY}" 'status,data.status,state,data.state' || true)"
    if [[ -n "${state}" ]]; then
      printf '[%s] 当前状态：%s\n' "${name}" "${state}"
    fi

    pretty_print_json "${RESPONSE_BODY}"
    printf '\n'

    if [[ "${RESPONSE_STATUS}" -ge 400 ]]; then
      printf '[%s] 轮询返回错误状态，停止轮询。\n\n' "${name}" >&2
      return 1
    fi

    normalized_state="$(printf '%s' "${state}" | tr '[:upper:]' '[:lower:]')"
    case "${normalized_state}" in
      completed|succeeded|success|failed|error|cancelled|canceled)
        printf '[%s] 任务进入终态：%s\n\n' "${name}" "${state}"
        return 0
        ;;
    esac

    if (( i < POLL_MAX_TIMES )); then
      sleep "${POLL_INTERVAL}"
    fi
  done

  printf '[%s] 达到最大轮询次数，停止轮询。\n\n' "${name}"
  return 0
}

submit_case() {
  local name="${1}"
  local channel_id="${2}"
  local payload="${3}"

  local auth_header
  auth_header="$(build_auth_header "${channel_id}")"

  printf '%s\n' '------------------------------------------------------------'
  printf '[%s] 提交请求\n' "${name}"
  if [[ -n "${channel_id}" ]]; then
    printf '[%s] 使用指定渠道后缀：%s\n' "${name}" "${channel_id}"
  else
    printf '[%s] 使用模型路由，不附加渠道后缀\n' "${name}"
  fi
  printf '[%s] POST %s/v1/video/generations\n' "${name}" "${NEWAPI_URL}"

  request_json "POST" "${NEWAPI_URL}/v1/video/generations" "${auth_header}" "${payload}"

  printf '[%s] HTTP %s\n' "${name}" "${RESPONSE_STATUS}"
  pretty_print_json "${RESPONSE_BODY}"
  printf '\n'

  if [[ "${RESPONSE_STATUS}" -ge 400 ]]; then
    printf '[%s] 提交失败。\n\n' "${name}" >&2
    return 1
  fi

  local task_id
  task_id="$(json_extract "${RESPONSE_BODY}" 'task_id,id,data.task_id,data.id' || true)"

  poll_task_if_possible "${name}" "${auth_header}" "${task_id}"
}

DOUBAO_PAYLOAD="$(cat <<'JSON'
{
  "model": "doubao-seedance-2-0",
  "content": [
    {
      "type": "text",
      "text": "一只机械鲸鱼跃出海面，电影感镜头，真实光影。"
    }
  ],
  "generate_audio": false,
  "ratio": "16:9",
  "duration": 5,
  "watermark": false
}
JSON
)"

CHENGMENG_PAYLOAD="$(cat <<'JSON'
{
  "model": "doubao-seedance-2-0",
  "prompt": "清晨薄雾中的湖边小屋，镜头缓慢推进，光线柔和。",
  "duration": 5,
  "images": [],
  "metadata": {
    "videos": [],
    "orientation": "landscape",
    "size": "720"
  }
}
JSON
)"

failures=0

if ! submit_case "DoubaoVideo" "${DOUBAO_CHANNEL_ID}" "${DOUBAO_PAYLOAD}"; then
  failures=$((failures + 1))
fi

if ! submit_case "Chengmeng" "${CHENGMENG_CHANNEL_ID}" "${CHENGMENG_PAYLOAD}"; then
  failures=$((failures + 1))
fi

if [[ "${failures}" -gt 0 ]]; then
  printf '完成：共有 %s 个用例失败。\n' "${failures}" >&2
  exit 1
fi

printf '完成：两个视频渠道测试请求均已执行。\n'
