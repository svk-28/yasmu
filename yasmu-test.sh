#!/usr/bin/env bash
# =============================================================================
# YASMU test suite — проверка всех типов мониторинга.
#
# Скрипт создаёт контролируемые изменения в системе и проверяет,
# что YASMU их фиксирует.
#
# ВАЖНО: запускать от root, в тестовой среде, НЕ на продакшене.
# Часть проверок создаёт временных пользователей, файлы, процессы.
#
# Использование:
#   sudo ./test-yasmu.sh [--keep] [--config /etc/yasmu/yasmu.yaml]
#
# Опции:
#   --keep     не удалять тестовые артефакты после прогона
#   --config   путь к конфигу YASMU (по умолчанию /etc/yasmu/yasmu.yaml)
# =============================================================================

set -u

# --- Цвета -------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# --- Флаги -------------------------------------------------------------------
KEEP=0
CONFIG="/etc/yasmu/yasmu.yaml"
YASMU_BIN="$(command -v yasmu || echo /usr/bin/yasmu)"
TEST_DIR="/tmp/yasmu-test-$$"
REPORT="/tmp/yasmu-test-report-$$.txt"

# --- Утилиты вывода ----------------------------------------------------------
log_info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok()    { echo -e "${GREEN}[ OK ]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_fail()  { echo -e "${RED}[FAIL]${NC} $*"; }

pass=0
fail=0

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local label="$3"

    if echo "$haystack" | grep -qF -- "$needle"; then
        log_ok "$label"
        pass=$((pass + 1))
    else
        log_fail "$label"
        echo "       ожидалось найти: $needle"
        echo "       в выводе:"
        echo "$haystack" | sed 's/^/         /' | head -20
        fail=$((fail + 1))
    fi
}

# --- Проверки предусловий ----------------------------------------------------
check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        log_fail "Скрипт нужно запускать от root"
        exit 1
    fi
}

check_binary() {
    if [ ! -x "$YASMU_BIN" ]; then
        log_fail "yasmu не найден: $YASMU_BIN"
        exit 1
    fi
    log_info "YASMU: $YASMU_BIN"
}

check_config() {
    if [ ! -f "$CONFIG" ]; then
        log_warn "Конфиг не найден: $CONFIG"
        log_info "Создаю временный конфиг с включёнными всеми проверками"

        CONFIG="$TEST_DIR/yasmu.yaml"
        mkdir -p "$TEST_DIR"
        cat > "$CONFIG" <<'EOF'
daemon:
  check_timeout: 5
  pid_file: /tmp/yasmu-test.pid
  log_file: /tmp/yasmu-test.log
  work_dir: /tmp

checks:
  files:
    enable: true
    paths:
      - path: /tmp/yasmu-test-$$
        watch: [create, modify, delete]
        hash: blake3
  users:
    enable: true
  processes:
    enable: true
    exclude:
      - kworker
  ports:
    enable: true
    established: false
  services:
    enable: true
  web:
    enable: false
  connections:
    enable: true
    rules:
      - name: "shell must not have network"
        proto: tcp
        state: ESTABLISHED
        processes: [bash, sh]
        alert_on_match: true
EOF
    fi
    log_info "Конфиг: $CONFIG"
}

# --- Запуск одной проверки ---------------------------------------------------
run_check() {
    "$YASMU_BIN" -config "$CONFIG" -run-once 2>&1
}

# =============================================================================
# ТЕСТЫ
# =============================================================================

test_files_create() {
    log_info "Тест: FilesCheck — создание файла"

    mkdir -p "$TEST_DIR/files"
    echo "hello" > "$TEST_DIR/files/new.txt"

    local out
    out=$(run_check)
    assert_contains "$out" "$TEST_DIR/files/new.txt" "files: создание зафиксировано"
}

test_files_modify() {
    log_info "Тест: FilesCheck — изменение содержимого"

    mkdir -p "$TEST_DIR/files"
    echo "v1" > "$TEST_DIR/files/mod.txt"
    run_check >/dev/null 2>&1   # первый обход — эталон

    echo "v2" > "$TEST_DIR/files/mod.txt"

    local out
    out=$(run_check)
    assert_contains "$out" "$TEST_DIR/files/mod.txt" "files: изменение зафиксировано"
}

test_files_delete() {
    log_info "Тест: FilesCheck — удаление файла"

    mkdir -p "$TEST_DIR/files"
    touch "$TEST_DIR/files/del.txt"
    run_check >/dev/null 2>&1   # эталон

    rm "$TEST_DIR/files/del.txt"

    local out
    out=$(run_check)
    assert_contains "$out" "$TEST_DIR/files/del.txt" "files: удаление зафиксировано"
}

test_files_hash() {
    log_info "Тест: FilesCheck — изменение при восстановленном mtime"

    mkdir -p "$TEST_DIR/files"
    echo "original" > "$TEST_DIR/files/hash.txt"
    local mtime
    mtime=$(stat -c %y "$TEST_DIR/files/hash.txt")
    run_check >/dev/null 2>&1   # эталон

    echo "tampered" > "$TEST_DIR/files/hash.txt"
    touch -d "$mtime" "$TEST_DIR/files/hash.txt"   # восстанавливаем mtime

    local out
    out=$(run_check)
    assert_contains "$out" "hash" "files: hash зафиксировал изменение (mtime восстановлен)"
}

test_processes_new() {
    log_info "Тест: ProcessesCheck — появление процесса"

    # Запускаем долгоживущий процесс
    sleep 300 &
    local pid=$!
    sleep 1

    local out
    out=$(run_check)
    assert_contains "$out" "PID $pid" "processes: новый процесс зафиксирован"

    kill "$pid" 2>/dev/null || true
}

test_processes_exclude() {
    log_info "Тест: ProcessesCheck — исключение kworker"

    # kworker — kernel thread, всегда есть
    local out
    out=$(run_check)
    if echo "$out" | grep -q "kworker"; then
        log_fail "processes: kworker не отфильтрован"
        fail=$((fail + 1))
    else
        log_ok "processes: kworker отфильтрован"
        pass=$((pass + 1))
    fi
}

test_services_add() {
    log_info "Тест: ServicesCheck — изменение состояния сервиса"

    # Используем systemd — запускаем/останавливаем существующий сервис
    # (осторожно: только безопасный, например, systemd-hostnamed)
    local unit="systemd-hostnamed.service"

    systemctl start "$unit" 2>/dev/null || true
    sleep 1
    run_check >/dev/null 2>&1   # эталон

    systemctl stop "$unit" 2>/dev/null || true
    sleep 1

    local out
    out=$(run_check)
    assert_contains "$out" "$unit" "services: изменение состояния зафиксировано"
}

test_ports_new() {
    log_info "Тест: PortsCheck — открытие порта"

    # Запускаем nc на порту 34567
    nc -l 34567 >/dev/null 2>&1 &
    local ncpid=$!
    sleep 1

    local out
    out=$(run_check)
    assert_contains "$out" "34567" "ports: новый порт зафиксирован"

    kill "$ncpid" 2>/dev/null || true
}

test_connections_bash() {
    log_info "Тест: ConnectionsCheck — bash с сетью"

    # nc-сервер
    nc -l 34568 >/dev/null 2>&1 &
    local ncpid=$!
    sleep 1

    # bash держит соединение через read -u
    bash -c 'exec 3<>/dev/tcp/127.0.0.1/34568; read -u 3 -t 30 line' &
    local bashpid=$!
    sleep 1

    local out
    out=$(run_check)
    assert_contains "$out" "shell must not have network" "connections: bash с сетью зафиксирован"

    kill "$bashpid" "$ncpid" 2>/dev/null || true
}

test_users_ssh_keys() {
    log_info "Тест: UsersCheck — изменение SSH-ключей"

    # Создаём тестового пользователя
    local test_user="yasmu-test-$$"
    if ! id "$test_user" >/dev/null 2>&1; then
        useradd -m -s /bin/bash "$test_user" 2>/dev/null || true
    fi

    local home
    home=$(getent passwd "$test_user" | cut -d: -f6)
    if [ -z "$home" ] || [ ! -d "$home" ]; then
        log_warn "Не удалось создать тестового пользователя — пропуск"
        return
    fi

    mkdir -p "$home/.ssh"
    chmod 700 "$home/.ssh"
    touch "$home/.ssh/authorized_keys"
    chown -R "$test_user:$test_user" "$home/.ssh"
    run_check >/dev/null 2>&1   # эталон

    echo "ssh-rsa AAAATESTKEY test@localhost" >> "$home/.ssh/authorized_keys"
    chown "$test_user:$test_user" "$home/.ssh/authorized_keys"

    local out
    out=$(run_check)
    assert_contains "$out" "$test_user" "users: изменение SSH-ключей зафиксировано"

    if [ "$KEEP" -eq 0 ]; then
        userdel -r "$test_user" 2>/dev/null || true
    fi
}

# =============================================================================
# MAIN
# =============================================================================

usage() {
    cat <<EOF
Использование: $0 [--keep] [--config PATH]

Опции:
  --keep            не удалять тестовые артефакты
  --config PATH     путь к конфигу YASMU
  -h, --help        эта справка
EOF
    exit 0
}

while [ $# -gt 0 ]; do
    case "$1" in
        --keep)   KEEP=1; shift ;;
        --config) CONFIG="$2"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "Неизвестная опция: $1"; usage ;;
    esac
done

check_root
check_binary
check_config

log_info "Тестовый каталог: $TEST_DIR"
mkdir -p "$TEST_DIR"

# Первый прогон — собираем эталон
log_info "Собираю эталон (первый обход)..."
run_check >/dev/null 2>&1

echo
echo "=========================================="
echo "  Запуск тестов YASMU"
echo "=========================================="
echo

test_files_create
test_files_modify
test_files_delete
test_files_hash
test_processes_new
test_processes_exclude
test_ports_new
test_connections_bash
test_services_add
test_users_ssh_keys

echo
echo "=========================================="
echo "  Итог: pass=$pass, fail=$fail"
echo "=========================================="

if [ "$KEEP" -eq 0 ]; then
    log_info "Удаляю тестовые артефакты"
    rm -rf "$TEST_DIR"
    rm -f /tmp/yasmu-test.log /tmp/yasmu-test.pid
else
    log_info "Артефакты сохранены: $TEST_DIR"
fi

if [ "$fail" -gt 0 ]; then
    exit 1
fi
exit 0
