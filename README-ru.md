# YASMU — Yet Another System Monitoring Utility

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

Легковесная утилита для мониторинга целостности системы и доступности веб-сайтов. Периодически собирает снимки состояния системы, сравнивает их с предыдущими и отправляет уведомления при обнаружении изменений.

---

## Оглавление

- [Возможности](#возможности)
- [Установка](#установка)
- [Быстрый старт](#быстрый-старт)
- [Командная строка](#командная-строка)
- [Конфигурация](#конфигурация)
- [Как это работает](#как-это-работает)
- [Модули (для разработчиков)](#модули-для-разработчиков)
- [Примеры использования](#примеры-использования)
- [Диагностика](#диагностика)
- [Файлы и каталоги](#файлы-и-каталоги)
- [Сборка пакетов](#сборка-пакетов)
- [Лицензия](#лицензия)
- [Ссылки](#ссылки)

---

## Возможности

### Мониторинг файловой системы

Позволяет отслеживать изменения в файловой системе, такие как создание, изменение, удаление файлов, BLAKE3-хеш содержимого. Через файл конфигурации можно указать какие каталоги отслеживать и какие операции по ним.

### Мониторинг пользователей

Программа читает список пользователей (локальный) и следит за созданием и удалением пользователей, изменением UID, домашнего каталога, группы.

Так же у каждого пользователя (как системного так и нет) проверяется наличие каталога .ssh в домашнем каталоге и SSH-ключей в authorized_keys. И отслеживаются изменения ключей.

### Мониторинг процессов

После запуска программы YASMU, проверяются все запущенные процессы и отслеживаются изменения идентификатора процесса (PID), идентификатора родительского процесса (PPID), владелеца, пути к бинарнику. Так же программа обращает внимание на удалённые бинарники у запущенных процессов.

### Мониторинг открытых портов (сокетов)

Отслеживаются сетевые порты: TCP 'LISTEN', UDP-сокеты, привязанные к локальному адресу.
Т.е. после запуска любые изменения в "слушающих" сетевых процессах (в заданных рамках) будут отслеживаться.

Через файл конфигурации можно задавать фильтры по портам и процессам.

### Мониторинг соединений

Мониторинг установленных соединений. Отслеживаются процессы установившие соединения с внешними (по отношению к целевому) узлами. 

Процесс описывается простыми правилами в файле конфигурации. Где можно задать на какие процессы реагировать, на какие нет, задать фильтры по адресам, портам, состоянию соединения и т.д.

### Мониторинг системных сервисов (systemd-юнитов)

В YASMU реализован мониторинг системных служб (через D-Bus) - текущее состояние, изменение состояния, т.е. включение, отключение службы, авариайное завершение работающего демона, запуск (LoadState, ActiveState, SubState, MainPID).

Можно следить как за всеми так и исключить из мониторинга некоторые сервисы, либо мониторить только требуемые. Это всё настраивается в файле конфигурации.

### Проверка доступности URL (web-проверка)

Программа опрашивает заданные в списке адреса и в случае ответа, отличного от 2xx, генерирует аварию (изменение статуса).

### Оповещение

По окончании цикла проверки программа может отправить результат по указанным координатам, путём отправки сообщения.

- E-mail, отправляет сообщение на заданный адрес через сервер электронной почты (smtp).
- Matrix, отправляет сообщение в заданный канал на указанном сервере посредством  протокола обмена сообщениями  Matrix.
- webhook, сообщение отправляется на любой http-ресурс. Сообщение может быть как текстовым так и в формате JSON (формат задаётся в конфиге).

### Режим работы

Программа YASMU может запускаться как в консоли, так и как служба (демон). 
При работе как службы, при остановке штатными средствами (системного менеджера, или опций командной строки yasmu), текущее состояние проверок будет сохранено на диск и при последующем старте загружено в память.

---

## Установка

### Debian / Ubuntu

1. Установить пакет:

    sudo apt install ./yasmu_1.0.0_amd64.deb

   Если `apt install ./file.deb` не поддерживается:

    sudo dpkg -i yasmu_1.0.0_amd64.deb
    sudo apt install -f

2. Отредактировать конфиг:

    sudo vi /etc/yasmu/config.yaml

3. Запустить сервис:

    sudo systemctl enable --now yasmu

4. Проверить:

    sudo systemctl status yasmu
    sudo journalctl -u yasmu -f

### RHEL / CentOS / Oracle Linux

1. Установить пакет:

    sudo dnf install ./yasmu-1.0.0-1.el8.x86_64.rpm

   Или:

    sudo rpm -ivh yasmu-1.0.0-1.el8.x86_64.rpm

2. Отредактировать конфиг:

    sudo vi /etc/yasmu/config.yaml

3. Запустить сервис:

    sudo systemctl enable --now yasmu

4. Проверить:

    sudo systemctl status yasmu
    sudo journalctl -u yasmu -f

### Из исходников

    git clone git.nuk-svk.ru/svk/yasmu.git
    cd yasmu
    make build
    sudo make install

---

## Быстрый старт

    # Создать конфиг по умолчанию
    yasmu -make-config -config /etc/yasmu/config.yaml

    # Отредактировать конфиг
    vi /etc/yasmu/yasmu.yaml

    # Запуск в консоли
    yasmu -config /etc/yasmu/yasmu.yaml

    # Запуск как демон
    yasmu -config /etc/yasmu/yasmu.yaml -daemon-start

    # Остановка демона
    yasmu -config /etc/yasmu/yasmu.yaml -daemon-stop

---

## Командная строка

| Флаг | Описание |
|---|---|
| `-config PATH` | Путь к файлу конфигурации. По умолчанию `config.yaml`. |
| `-run-once` | Выполнить все проверки один раз и выйти. |
| `-daemon-start` | Запустить как демон. |
| `-daemon-stop` | Остановить демон. |
| `-make-config` | Создать файл конфигурации из шаблона. |
| `-version` | Показать версию и выйти. |
| `-h`, `-help` | Показать справку. |

---

## Конфигурация

### Секция `daemon`

| Поле | Тип | Описание |
|---|---|---|
| `check_timeout` | int | Пауза между итерациями, в секундах. `<= 0` → 30 секунд. |
| `pid_file` | string | Путь к PID-файлу. |
| `log_file` | string | Путь к лог-файлу. |
| `work_dir` | string | Рабочий каталог демона. |

    daemon:
      check_timeout: 60
      pid_file: /run/yasmu/yasmu.pid
      log_file: /var/log/yasmu/yasmu.log
      work_dir: /var/lib/yasmu

### Секция `checks`

Каждая подсекция — отдельный модуль. Флаг `enable: false` пропускает проверку.

#### `checks.files`

    checks:
      files:
        enable: true
        paths:
          - path: /etc
            watch: [create, modify, delete]
            hash: blake3
            exclude:
              - /etc/mtab

| Поле | Тип | Описание |
|---|---|---|
| `path` | string | Корень обхода (рекурсивно). |
| `watch` | []string | `create`, `modify`, `delete`. Пусто — все. |
| `hash` | string | `blake3`, `sha256` или пусто. |
| `exclude` | []string | Подпути для пропуска. |

Сравнивается: тип, размер, права, владелец, группа, mtime, хеш.

#### `checks.users`

    checks:
      users:
        enable: true

Сравнивается: UID, primary GID, home, группы, `~/.ssh/authorized_keys`.

#### `checks.processes`

    checks:
      processes:
        enable: true
        exclude:
          - kworker
          - /opt/zimbra

| Поле | Тип | Описание |
|---|---|---|
| `exclude` | []string | Подстроки в имени или Exe. |

Сравнивается: PID, PPID, Name, Owner, Exe, Deleted, Suspicious.

#### `checks.ports`

    checks:
      ports:
        enable: true
        established: false
        exclude_ports: [10024]
        exclude_processes: [/opt/zimbra]
        ignore_pid_changes: false

| Поле | Тип | Описание |
|---|---|---|
| `established` | bool | Включать ESTABLISHED TCP. |
| `exclude_ports` | []uint32 | Порты для пропуска. |
| `exclude_processes` | []string | Подстроки в Exe. |
| `ignore_pid_changes` | bool | Игнорировать смену PID/Exe. |

#### `checks.services`

    checks:
      services:
        enable: true
        units: [sshd.service, nginx.service]
        exclude: [NetworkManager-dispatcher.service]

| Поле | Тип | Описание |
|---|---|---|
| `units` | []string | Явный список. Пусто → все. |
| `exclude` | []string | Подстроки для пропуска. |

Сравнивается: LoadState, ActiveState, SubState, MainPID.

#### `checks.connections`

    checks:
      connections:
        enable: true
        rules:
          - name: "SSH from allowed networks"
            proto: tcp
            port: 22
            state: ESTABLISHED
            allowed_remote: [192.168.0.0/16]
          - name: "shell must not have network"
            proto: tcp
            state: ESTABLISHED
            processes: [bash, sh]
            alert_on_match: true

| Поле правила | Тип | Описание |
|---|---|---|
| `name` | string | Имя правила. |
| `proto` | string | tcp / udp. |
| `port` | uint32 | Локальный порт. |
| `state` | string | TCP-состояние. |
| `processes` | []string | Точные basename. |
| `allowed_remote` | []string | Whitelist IP/CIDR. |
| `alert_on_match` | bool | Инвертирует логику. |

#### `checks.web`

    checks:
      web:
        enable: true
        urls:
          - https://server.com
          - https://git.server.com

### Секция `notify`

#### `notify.email`

    notify:
      email:
        enable: true
        server: smtp.yandex.ru
        port: 465
        user: user@yandex.ru
        password: "app-password"
        mail_to: admin@domain.com
        mail_from: user@yandex.ru
        mail_subject: "YASMU alert from %HOSTNAME%"

Порты 465 (implicit TLS) и 587 (STARTTLS).

#### `notify.matrix`

    notify:
      matrix:
        enable: true
        server: https://matrix.example.com
        user: "@user:example.com"
        password: "password"
        access_token: ""
        room_id: "!abc:example.com"

#### `notify.webhook`

    notify:
      webhook:
        enable: true
        url: https://web.server.com/hook
        user: user
        password: "password"
        message:
          format: json
          template: '{"message":"Alert from %HOSTNAME%: %MESSAGE%"}'

Макросы: `%HOSTNAME%`, `%MESSAGE%`.

---

## Примеры использования

### Минимальный конфиг

    daemon:
      check_timeout: 300
      work_dir: /var/lib/yasmu

    checks:
      files:
        enable: true
        paths:
          - path: /etc
            watch: [modify, delete]
            hash: blake3

    notify:
      email:
        enable: true
        server: smtp.example.com
        port: 587
        user: alerts@example.com
        password: "secret"
        mail_to: admin@example.com
        mail_from: alerts@example.com
        mail_subject: "YASMU: %HOSTNAME%"

### Systemd-юнит

    [Unit]
    Description=YASMU - Yet Another System Monitoring Utility
    After=network.target

    [Service]
    Type=simple
    ExecStart=/usr/bin/yasmu -config /etc/yasmu/yasmu.yaml -daemon
    Restart=always
    RestartSec=10

    [Install]
    WantedBy=multi-user.target

### Управление

    sudo systemctl start yasmu
    sudo systemctl stop yasmu
    sudo systemctl status yasmu
    sudo systemctl restart yasmu
    sudo journalctl -u yasmu -f

---

## Диагностика

### Логи

- Systemd: journalctl -u yasmu -f
- Файл: /var/log/yasmu.log

### Типичные проблемы

### Отладка

---

## Файлы и каталоги

- /usr/bin/yasmu - запрускаемый файл
- /etc/yasmu/config.yaml - файл конфигурации
- /lib/systemd/system/yasmu.service - Systemd-юнит (Debian)
- /usr/lib/systemd/system/yasmu.service - Systemd-юнит (RPM)
- /var/lib/yasmu/ - Состояние
- /var/log/yasmu.log - Журнал
- /run/yasmu.pid - PID-файл

---

## Сборка пакетов

### Debian (.deb)

    make deb
    sudo dpkg -i build/yasmu_1.0.0_amd64.deb

### RHEL / CentOS / Oracle (.rpm)

    make rpm
    sudo rpm -ivh ~/rpmbuild/RPMS/x86_64/yasmu-1.0.0-1.el8.x86_64.rpm

### Статический бинарник

    make build
    # или
	go build -ldflags "-s -w" -o bin/yasmu main.go config.go
  	
### Статический бинарник

    make static
    # или
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -ldflags "-s -w" -o bin/yasmu main.go config.go

---

## Лицензия

GPL v3. См. файл LICENSE в репозитории.

---

## Ссылки

- Home page: https://nuk-svk.ru

