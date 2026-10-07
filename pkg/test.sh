#!/usr/bin/env bash
# =============================================================================
# YASMU functional test — для УЖЕ ЗАПУЩЕННОГО демона
#
# Читает /etc/yasmu/config.yml, триггерит события в реальных путях,
# проверяет лог YASMU на срабатывание.
#
# ВАЖНО:
#   - Запускать от root
#   - ТОЛЬКО в тестовой среде
#   - YASMU должен быть запущен
# =============================================================================

set -u

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'

TEST_ID="$$"
CONFIG="/etc/yasmu/config.yaml"
LOG_FILE=""
USE_JOURNAL=0
CHECK_TIMEOUT=60
WAIT_AFTER=""
NC_LISTEN=""

pass=0
fail=0

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok()   { echo -e "${GREEN}[ OK ]${NC} $*"; }
log_fail() { echo -e "${RED}[FAIL]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_step() { echo; echo -e "${BLUE}=== $* ===${NC}"; }

# =============================================================================
# NC
# =============================================================================

detect_nc() {
    if ! command -v nc >/dev/null 2>&1; then
        NC_LISTEN=""
        return
    fi
    if nc -h 2>&1 | grep -q "v1.10\|traditional"; then
        NC_LISTEN="nc -l -p"
        log_info "nc: traditional"
    else
        NC_LISTEN="nc -l"
        log_info "nc: openbsd"
    fi
}

start_tcp_server() {
    local port="$1"
    if [ -n "$NC_LISTEN" ]; then
        $NC_LISTEN "$port" >/dev/null 2>&1 &
    else
        python3 -c "
import socket, time
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('0.0.0.0', $port))
s.listen(1)
time.sleep(120)
" >/dev/null 2>&1 &
    fi
    echo $!
}

port_is_listening() {
    ss -tln 2>/dev/null | grep -q ":$1 "
}

kill_port() {
    fuser -k "$1/tcp" 2>/dev/null || true
}

# =============================================================================
# КОНФИГ
# =============================================================================

read_config() {
    log_info "Читаю конфиг: $CONFIG"

    [ -f "$CONFIG" ] || { log_fail "Конфиг не найден: $CONFIG"; exit 1; }

    local t
    t=$(grep -E "^\s*check_timeout:" "$CONFIG" | head -1 | awk '{print $2}')
    [ -n "$t" ] && CHECK_TIMEOUT="$t"
    log_info "check_timeout: ${CHECK_TIMEOUT}s"

    local lf
    lf=$(grep -E "^\s*log_file:" "$CONFIG" | head -1 | awk '{print $2}')

    if [ -n "$lf" ]; then
        if [ -f "$lf" ]; then
            LOG_FILE="$lf"
        else
            local wd
            wd=$(grep -E "^\s*work_dir:" "$CONFIG" | head -1 | awk '{print $2}')
            if [ -n "$wd" ] && [ -f "${wd%/}/${lf}" ]; then
                LOG_FILE="${wd%/}/${lf}"
            fi
        fi
    fi

    if [ -n "$LOG_FILE" ] && [ -f "$LOG_FILE" ]; then
        log_info "log_file: $LOG_FILE"
    else
        USE_JOURNAL=1
        log_info "log_file не найден — читаю journalctl"
    fi
}

read_log() {
    local since_sec="${1:-$WAIT_AFTER}"
    if [ "$USE_JOURNAL" -eq 1 ]; then
        journalctl -u yasmu --since "${since_sec} seconds ago" --no-pager 2>/dev/null
    else
        tail -500 "$LOG_FILE" 2>/dev/null
    fi
}

assert_in_log() {
    local needle="$1"; local label="$2"; local out="$3"
    if echo "$out" | grep -qF -- "$needle"; then
        log_ok "$label"
        pass=$((pass + 1))
    else
        log_fail "$label"
        echo "       ожидалось: $needle"
        echo "       последние 25 строк:"
        echo "$out" | tail -25 | sed 's/^/         /'
        fail=$((fail + 1))
    fi
}

wait_and_check() {
    local needle="$1"; local label="$2"
    log_info "Ждём ${WAIT_AFTER}с (check_timeout=${CHECK_TIMEOUT}с)..."
    sleep "$WAIT_AFTER"
    local out
    out=$(read_log "$WAIT_AFTER")
    assert_in_log "$needle" "$label" "$out"
}

# Извлекает первый files.path из конфига (устойчиво к отступам)
get_first_files_path() {
    grep -E "^\s*-\s*path:\s+\S" "$CONFIG" | head -1 | \
        sed -E 's/^\s*-\s*path:\s*//; s/\s.*$//'
}

# =============================================================================
# ТЕСТЫ
# =============================================================================

TEST_FILES_PATH=""

test_files_create() {
    log_step "FilesCheck: создание файла"

    [ -z "$TEST_FILES_PATH" ] && { log_warn "files.paths пуст — пропуск"; return; }

    local f="${TEST_FILES_PATH}/yasmu-test-new-${TEST_ID}.txt"
    log_info "Создаю: $f"
    echo "hello" > "$f" 2>/dev/null || {
        log_fail "Не могу создать $f (права?)"
        fail=$((fail + 1)); return
    }

    wait_and_check "yasmu-test-new-${TEST_ID}.txt" "files: создание зафиксировано"
}

test_files_modify() {
    log_step "FilesCheck: изменение содержимого"

    [ -z "$TEST_FILES_PATH" ] && { log_warn "files.paths пуст — пропуск"; return; }

    local f="${TEST_FILES_PATH}/yasmu-test-mod-${TEST_ID}.txt"
    echo "v1" > "$f"
    log_info "Создан $f, ждём фиксации"
    sleep "$WAIT_AFTER"

    echo "v2" > "$f"
    log_info "Изменён $f"

    wait_and_check "yasmu-test-mod-${TEST_ID}.txt" "files: изменение зафиксировано"
}

test_files_delete() {
    log_step "FilesCheck: удаление файла"

    [ -z "$TEST_FILES_PATH" ] && { log_warn "files.paths пуст — пропуск"; return; }

    local f="${TEST_FILES_PATH}/yasmu-test-del-${TEST_ID}.txt"
    touch "$f"
    log_info "Создан $f, ждём фиксации"
    sleep "$WAIT_AFTER"

    rm "$f"
    log_info "Удалён $f"

    wait_and_check "yasmu-test-del-${TEST_ID}.txt" "files: удаление зафиксировано"
}

test_processes_new() {
    log_step "ProcessesCheck: появление процесса"

    # Процесс с уникальным именем, чтобы не отфильтровался
    sleep 120 &
    local pid=$!
    log_info "Запущен sleep, PID=$pid"

    wait_and_check "PID $pid" "processes: новый процесс зафиксирован"

    kill "$pid" 2>/dev/null || true
}

test_ports_new() {
    log_step "PortsCheck: открытие нового порта"

    local port=34567

    kill_port "$port"
    sleep 1

    if port_is_listening "$port"; then
        log_fail "Порт $port занят"
        fail=$((fail + 1)); return
    fi

    local server_pid
    server_pid=$(start_tcp_server "$port")
    sleep 2

    if ! port_is_listening "$port"; then
        log_fail "Сервер не слушает $port"
        kill "$server_pid" 2>/dev/null || true
        fail=$((fail + 1)); return
    fi

    log_info "TCP-сервер слушает $port (PID $server_pid)"

    wait_and_check "$port" "ports: новый порт зафиксирован"

    kill "$server_pid" 2>/dev/null || true
    kill_port "$port"
}

test_connections_bash() {
    log_step "ConnectionsCheck: bash с сетью"

    local port=34568

    kill_port "$port"
    sleep 1

    if port_is_listening "$port"; then
        log_fail "Порт $port занят"
        fail=$((fail + 1)); return
    fi

    local server_pid
    server_pid=$(start_tcp_server "$port")
    sleep 2

    if ! port_is_listening "$port"; then
        log_fail "Сервер не слушает $port"
        kill "$server_pid" 2>/dev/null || true
        fail=$((fail + 1)); return
    fi

    # bash держит соединение через /dev/tcp
    bash -c "exec 3<>/dev/tcp/127.0.0.1/$port; read -u 3 -t 120 line" &
    local bashpid=$!
    sleep 2

    if ! ss -tn 2>/dev/null | grep -q "127.0.0.1:$port "; then
        log_fail "bash не установил соединение с $port"
        kill "$bashpid" "$server_pid" 2>/dev/null || true
        fail=$((fail + 1)); return
    fi

    log_info "bash держит соединение с $port (PID $bashpid)"

    wait_and_check "shell must not have network" "connections: bash зафиксирован"

    kill "$bashpid" "$server_pid" 2>/dev/null || true
    kill_port "$port"
}

test_users_ssh_keys() {
    log_step "UsersCheck: изменение SSH-ключей"

    local test_user="yasmu-test-${TEST_ID}"

    id "$test_user" >/dev/null 2>&1 && userdel -r "$test_user" 2>/dev/null || true
    useradd -m -s /bin/bash "$test_user" 2>/dev/null || true

    local home
    home=$(getent passwd "$test_user" | cut -d: -f6)
    if [ -z "$home" ] || [ ! -d "$home" ]; then
        log_warn "users: не удалось создать пользователя — пропуск"
        return
    fi

    mkdir -p "$home/.ssh"
    chmod 700 "$home/.ssh"
    touch "$home/.ssh/authorized_keys"
    chown -R "$test_user:$test_user" "$home/.ssh"

    log_info "Создан $test_user, ждём фиксации"
    sleep "$WAIT_AFTER"

    echo "ssh-rsa AAAATESTKEY test@localhost" >> "$home/.ssh/authorized_keys"
    chown "$test_user:$test_user" "$home/.ssh/authorized_keys"
    log_info "Добавлен SSH-ключ"

    wait_and_check "$test_user" "users: SSH-ключ зафиксирован"

    userdel -r "$test_user" 2>/dev/null || true
}

# =============================================================================
# MAIN
# =============================================================================

usage() {
    cat <<EOF
Использование: $0 [--config PATH] [--wait SEC]

Опции:
  --config PATH   конфиг YASMU (по умолчанию /etc/yasmu/config.yml)
  --wait SEC      сколько ждать после триггера (по умолчанию check_timeout*2+10)
  -h, --help      эта справка
EOF
    exit 0
}

while [ $# -gt 0 ]; do
    case "$1" in
        --config) CONFIG="$2"; shift 2 ;;
        --wait)   WAIT_AFTER="$2"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "Неизвестная опция: $1"; usage ;;
    esac
done

[ "$(id -u)" -ne 0 ] && { log_fail "Нужен root"; exit 1; }

detect_nc
read_config

[ -z "$WAIT_AFTER" ] && WAIT_AFTER=$((CHECK_TIMEOUT * 2 + 10))
log_info "WAIT_AFTER: ${WAIT_AFTER}s"

if ! pgrep -x yasmu >/dev/null 2>&1; then
    if systemctl is-active --quiet yasmu 2>/dev/null; then
        log_info "Служба yasmu активна"
    else
        log_fail "YASMU не запущен"
        exit 1
    fi
fi

TEST_FILES_PATH="$(get_first_files_path)"
log_info "Первый files.path: ${TEST_FILES_PATH:-<пусто>}"

[ -z "$TEST_FILES_PATH" ] && log_warn "files.paths пуст — тесты files пропущены"

echo
echo "=========================================="
echo "  Запуск тестов (YASMU уже работает)"
echo "  check_timeout: ${CHECK_TIMEOUT}s"
echo "  WAIT_AFTER:    ${WAIT_AFTER}s"
echo "=========================================="

test_files_create
test_files_modify
test_files_delete
test_processes_new
test_ports_new
test_connections_bash
test_users_ssh_keys

echo
echo "=========================================="
echo "  Итог: pass=$pass, fail=$fail"
echo "=========================================="

[ "$fail" -gt 0 ] && exit 1
exit 0

