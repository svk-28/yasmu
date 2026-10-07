#!/usr/bin/env bash
# =============================================================================
# YASMU functional test — для УЖЕ ЗАПУЩЕННОГО демона
#
# Читает /etc/yasmu/config.yaml, триггерит события в реальных путях,
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
TEST_FILES_PATH=""
TEST_SERVICE_NAME=""

pass=0
fail=0

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok()   { echo -e "${GREEN}[ OK ]${NC} $*"; }
log_fail() { echo -e "${RED}[FAIL]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_step() { echo; echo -e "${BLUE}=== $* ===${NC}"; }

# =============================================================================
# CLEANUP
# =============================================================================

cleanup() {
    remove_test_service 2>/dev/null || true
    userdel -r "yasmu-test-${TEST_ID}" 2>/dev/null || true
    rm -f "/tmp/yasmu-test-bin-${TEST_ID}" 2>/dev/null || true
    rm -f "${TEST_FILES_PATH}/yasmu-test-new-${TEST_ID}.txt" 2>/dev/null || true
    rm -f "${TEST_FILES_PATH}/yasmu-test-mod-${TEST_ID}.txt" 2>/dev/null || true
    rm -f "${TEST_FILES_PATH}/yasmu-test-del-${TEST_ID}.txt" 2>/dev/null || true
    kill_port 34567 2>/dev/null || true
    kill_port 34568 2>/dev/null || true
    kill_port 34569 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# =============================================================================
# NC / TCP HELPERS
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
        tail -1000 "$LOG_FILE" 2>/dev/null
    fi
}

# =============================================================================
# ПРОВЕРКИ
# =============================================================================

# Проверяет, включена ли секция конфига (enable: true).
# Ищет "  <section>:" на уровне 2 пробелов, потом первый enable: внутри.
section_enabled() {
    local section="$1"
    awk -v sec="$section" '
        $0 ~ "^  "sec":" { in_sec=1; next }
        in_sec && /^  [a-z]/ { exit }
        in_sec && /enable:[ \t]*true/ { print "yes"; exit }
    ' "$CONFIG" | grep -q yes
}

assert_yasmu_alive() {
    if ! pgrep -x yasmu >/dev/null 2>&1 && ! systemctl is-active --quiet yasmu 2>/dev/null; then
        log_fail "YASMU умер во время теста"
        exit 1
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

# Ждёт сработки с ретраями: не один sleep, а периодические проверки лога.
wait_and_check() {
    local needle="$1"; local label="$2"
    local deadline=$(( $(date +%s) + WAIT_AFTER + CHECK_TIMEOUT ))
    log_info "Ждём сработки (до $((WAIT_AFTER + CHECK_TIMEOUT))с)..."

    while [ "$(date +%s)" -lt "$deadline" ]; do
        assert_yasmu_alive
        local out
        out=$(read_log "$WAIT_AFTER")
        if echo "$out" | grep -qF -- "$needle"; then
            log_ok "$label"
            pass=$((pass + 1))
            return
        fi
        sleep 3
    done

    log_fail "$label"
    local out
    out=$(read_log "$WAIT_AFTER")
    echo "       ожидалось: $needle"
    echo "$out" | tail -25 | sed 's/^/         /'
    fail=$((fail + 1))
}

# Извлекает первый files.path из конфига (устойчиво к отступам)
get_first_files_path() {
    grep -E "^\s*-\s*path:\s+\S" "$CONFIG" | head -1 | \
        sed -E 's/^\s*-\s*path:\s*//; s/\s.*$//'
}

# =============================================================================
# ТЕСТЫ: FILES
# =============================================================================

test_files_create() {
    log_step "FilesCheck: создание файла"

    if ! section_enabled "files"; then
        log_warn "checks.files.enable != true — пропуск"
        return
    fi
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

    if ! section_enabled "files"; then
        log_warn "checks.files.enable != true — пропуск"
        return
    fi
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

    if ! section_enabled "files"; then
        log_warn "checks.files.enable != true — пропуск"
        return
    fi
    [ -z "$TEST_FILES_PATH" ] && { log_warn "files.paths пуст — пропуск"; return; }

    local f="${TEST_FILES_PATH}/yasmu-test-del-${TEST_ID}.txt"
    touch "$f"
    log_info "Создан $f, ждём фиксации"
    sleep "$WAIT_AFTER"

    rm "$f"
    log_info "Удалён $f"

    wait_and_check "yasmu-test-del-${TEST_ID}.txt" "files: удаление зафиксировано"
}

# =============================================================================
# ТЕСТЫ: PROCESSES
# =============================================================================

test_processes_new() {
    log_step "ProcessesCheck: появление процесса"

    if ! section_enabled "processes"; then
        log_warn "checks.processes.enable != true — пропуск"
        return
    fi

    sleep 120 &
    local pid=$!
    log_info "Запущен sleep, PID=$pid"

    wait_and_check "PID $pid" "processes: новый процесс зафиксирован"

    kill "$pid" 2>/dev/null || true
}

test_processes_stop() {
    log_step "ProcessesCheck: остановка процесса"

    if ! section_enabled "processes"; then
        log_warn "checks.processes.enable != true — пропуск"
        return
    fi

    sleep 120 &
    local pid=$!
    log_info "Запущен sleep, PID=$pid, ждём фиксации"
    sleep "$WAIT_AFTER"

    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
    log_info "Убит PID $pid"

    wait_and_check "PID $pid" "processes: остановка зафиксирована"
}

test_processes_deleted_exe() {
    log_step "ProcessesCheck: удалённый бинарник"

    if ! section_enabled "processes"; then
        log_warn "checks.processes.enable != true — пропуск"
        return
    fi

    local tmp_bin="/tmp/yasmu-test-bin-${TEST_ID}"
    cp /bin/sleep "$tmp_bin" 2>/dev/null || {
        log_warn "Не могу скопировать /bin/sleep — пропуск"
        return
    }

    "$tmp_bin" 120 &
    local pid=$!
    log_info "Запущен $tmp_bin (PID $pid), ждём фиксации"
    sleep "$WAIT_AFTER"

    rm -f "$tmp_bin"
    log_info "Удалён бинарник $tmp_bin"

    wait_and_check "deleted" "processes: удалённый бинарник зафиксирован"

    kill "$pid" 2>/dev/null || true
}

# =============================================================================
# ТЕСТЫ: PORTS
# =============================================================================

test_ports_new() {
    log_step "PortsCheck: открытие нового порта"

    if ! section_enabled "ports"; then
        log_warn "checks.ports.enable != true — пропуск"
        return
    fi

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

test_ports_close() {
    log_step "PortsCheck: закрытие порта"

    if ! section_enabled "ports"; then
        log_warn "checks.ports.enable != true — пропуск"
        return
    fi

    local port=34569
    kill_port "$port"
    sleep 1

    local server_pid
    server_pid=$(start_tcp_server "$port")
    sleep 2

    if ! port_is_listening "$port"; then
        log_fail "Сервер не слушает $port"
        kill "$server_pid" 2>/dev/null || true
        fail=$((fail + 1)); return
    fi

    log_info "Сервер слушает $port, ждём фиксации"
    sleep "$WAIT_AFTER"

    kill "$server_pid" 2>/dev/null || true
    kill_port "$port"
    log_info "Сервер остановлен, порт $port закрыт"

    wait_and_check "$port" "ports: закрытие зафиксировано"
}

# =============================================================================
# ТЕСТЫ: CONNECTIONS
# =============================================================================

test_connections_bash() {
    log_step "ConnectionsCheck: bash с сетью"

    if ! section_enabled "connections"; then
        log_warn "checks.connections.enable != true — пропуск"
        return
    fi

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

# =============================================================================
# ТЕСТЫ: SERVICES (systemd)
# =============================================================================

create_test_service() {
    TEST_SERVICE_NAME="yasmu-test-${TEST_ID}.service"
    local unit_path="/etc/systemd/system/${TEST_SERVICE_NAME}"

    cat > "$unit_path" <<EOF
[Unit]
Description=YASMU test service (safe to remove)

[Service]
Type=simple
ExecStart=/bin/sleep 600
Restart=no

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload 2>/dev/null || true
    log_info "Создан unit: $TEST_SERVICE_NAME"
}

remove_test_service() {
    [ -z "$TEST_SERVICE_NAME" ] && return
    systemctl stop "$TEST_SERVICE_NAME" 2>/dev/null || true
    systemctl disable "$TEST_SERVICE_NAME" 2>/dev/null || true
    rm -f "/etc/systemd/system/${TEST_SERVICE_NAME}" 2>/dev/null || true
    systemctl daemon-reload 2>/dev/null || true
    log_info "Удалён unit: $TEST_SERVICE_NAME"
    TEST_SERVICE_NAME=""
}

test_services_start() {
    log_step "ServicesCheck: запуск нового сервиса"

    if ! section_enabled "services"; then
        log_warn "checks.services.enable != true — пропуск"
        return
    fi

    create_test_service
    log_info "Запускаю $TEST_SERVICE_NAME"

    systemctl start "$TEST_SERVICE_NAME" 2>/dev/null || {
        log_fail "Не удалось запустить $TEST_SERVICE_NAME"
        remove_test_service
        fail=$((fail + 1))
        return
    }

    wait_and_check "$TEST_SERVICE_NAME" "services: запуск зафиксирован"

    systemctl stop "$TEST_SERVICE_NAME" 2>/dev/null || true
    remove_test_service
}

test_services_stop() {
    log_step "ServicesCheck: остановка сервиса"

    if ! section_enabled "services"; then
        log_warn "checks.services.enable != true — пропуск"
        return
    fi

    create_test_service
    systemctl start "$TEST_SERVICE_NAME" 2>/dev/null || true
    sleep 1

    log_info "Останавливаю $TEST_SERVICE_NAME"
    systemctl stop "$TEST_SERVICE_NAME" 2>/dev/null || true

    wait_and_check "$TEST_SERVICE_NAME" "services: остановка зафиксирована"

    remove_test_service
}

test_services_restart() {
    log_step "ServicesCheck: перезапуск сервиса (смена MainPID)"

    if ! section_enabled "services"; then
        log_warn "checks.services.enable != true — пропуск"
        return
    fi

    create_test_service
    systemctl start "$TEST_SERVICE_NAME" 2>/dev/null || true
    sleep 1

    local pid_before
    pid_before=$(systemctl show -p MainPID --value "$TEST_SERVICE_NAME" 2>/dev/null)
    log_info "MainPID до перезапуска: $pid_before"

    systemctl restart "$TEST_SERVICE_NAME" 2>/dev/null || true
    sleep 1

    local pid_after
    pid_after=$(systemctl show -p MainPID --value "$TEST_SERVICE_NAME" 2>/dev/null)
    log_info "MainPID после перезапуска: $pid_after"

    if [ "$pid_before" = "$pid_after" ]; then
        log_warn "MainPID не изменился — сервис мог не перезапуститься"
    fi

    wait_and_check "$TEST_SERVICE_NAME" "services: перезапуск зафиксирован"

    systemctl stop "$TEST_SERVICE_NAME" 2>/dev/null || true
    remove_test_service
}

# =============================================================================
# ТЕСТЫ: USERS
# =============================================================================

test_users_ssh_keys() {
    log_step "UsersCheck: изменение SSH-ключей"

    if ! section_enabled "users"; then
        log_warn "checks.users.enable != true — пропуск"
        return
    fi

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
# ТЕСТЫ: WEB (заглушка)
# =============================================================================

test_web_unavailable() {
    log_step "WebCheck: недоступный URL"

    if ! section_enabled "web"; then
        log_warn "checks.web.enable != true — пропуск"
        return
    fi

    local url
    url=$(grep -E "^\s*-\s*https?://" "$CONFIG" | head -1 | awk '{print $2}')
    if [ -z "$url" ]; then
        log_warn "web.urls пуст — пропуск"
        return
    fi

    if ! curl -sf -o /dev/null --max-time 5 "$url"; then
        log_warn "URL $url уже недоступен — тест неинформативен, пропуск"
        return
    fi

    log_warn "web: тест недоступности требует ручной настройки — пропуск"
}

# =============================================================================
# MAIN
# =============================================================================

usage() {
    cat <<EOF
Использование: $0 [--config PATH] [--wait SEC]

Опции:
  --config PATH   конфиг YASMU (по умолчанию /etc/yasmu/config.yaml)
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
test_processes_stop
test_processes_deleted_exe
test_ports_new
test_ports_close
test_connections_bash
test_services_start
test_services_stop
test_services_restart
test_users_ssh_keys
test_web_unavailable

echo
echo "=========================================="
echo "  Итог: pass=$pass, fail=$fail"
echo "=========================================="

[ "$fail" -gt 0 ] && exit 1
exit 0
