<div align="center">

# ⚡ SubForge

**Панель управления VPN-подписками**  
Работает напрямую с бинарниками xray-core и hysteria2, без промежуточных панелей.

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/sssilverhand/subforge)](https://github.com/sssilverhand/subforge/releases)

[🇬🇧 Read in English](README.md) · [Issues](https://github.com/sssilverhand/subforge/issues) · [Releases](https://github.com/sssilverhand/subforge/releases)

</div>

---

## Возможности

- **Смешанные протоколы** — VLESS (XHTTP / Reality / WS) + Hysteria2 в одной ссылке подписки
- **Мультинода** — добавляй неограниченное количество VPS, клиент получает конфиги со всех нод
- **Форматы подписок** — Sing-box JSON, Clash Meta YAML, raw base64 (выбирается через `?client=`)
- **Контроль доступа в реальном времени** — xray через gRPC API (без перезапуска), hysteria2 через HTTP auth backend
- **Лимиты трафика и времени** — жёсткое отключение при достижении лимита, фоновый планировщик каждые 60 сек
- **Telegram бот** — управление для админа + личный кабинет для пользователей (статус, ссылки по клиентам)
- **Оплата** — Cryptomus (крипто) и Telegram Stars, опциональный режим автоматического создания подписок
- **Агент ноды** — установка и настройка xray/hysteria2 прямо из панели
- **Единый бинарник** — Go бэкенд + React интерфейс внутри, запускается как systemd-сервис

## Установка

```bash
bash <(curl -Ls https://raw.githubusercontent.com/sssilverhand/subforge/main/install.sh)
```

Скрипт автоматически:
- Установит PostgreSQL
- Скачает последний релиз (Go и Node.js **не нужны**)
- Настроит конфиг и systemd-сервис
- Запустит SubForge

## Первый запуск

После установки создай администратора:

```bash
curl -X POST http://localhost:8080/api/setup \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"your-password"}'
```

Затем в `/etc/subforge/config.yaml` выставь `super_admin_setup: false` и:

```bash
systemctl restart subforge
```

Открой браузер: `http://your-server-ip:8080`

## Архитектура

```
Браузер / Telegram Бот
        │
   SubForge (Go)             ← JWT аутентификация, REST API, раздача подписок
        │
   PostgreSQL
        │
   Агент ноды (Go)           ← на каждом VPS
   ├── xray gRPC API
   └── hysteria2 HTTP API
```

## Настройка hysteria2

SubForge выступает auth-бэкендом для hysteria2. Добавь в конфиг hysteria2:

```yaml
auth:
  type: http
  http:
    url: http://127.0.0.1:8080/internal/hy2/auth

trafficStats:
  listen: 127.0.0.1:11451
  secret: your-hy2-secret
```

В настройках ноды в панели укажи `hy2_api_url` и `hy2_api_secret`.

## Telegram бот

1. Создай бота через [@BotFather](https://t.me/BotFather), скопируй токен
2. В `/etc/subforge/config.yaml`:

```yaml
bot:
  enabled: true
  token: "123456:ABC..."
```

3. В панели → Users → укажи свой Telegram chat ID
4. Напиши `/start` боту — получишь меню администратора

Пользователи пишут `/start` боту — регистрируются автоматически. Ты видишь их в панели и выдаёшь подписки вручную или через автоматическую оплату.

## Режим оплаты

Включается в конфиге:

```yaml
bot:
  payment_mode: true
  payment_provider: "cryptomus"  # или telegram_stars
```

При включении бот показывает кнопку «Купить подписку» → пользователь выбирает тариф → оплачивает → подписка создаётся автоматически.

## Агент ноды (опционально)

Позволяет устанавливать и обновлять xray/hysteria2 прямо из панели.

```bash
# На каждом VPS:
curl -fsSL https://raw.githubusercontent.com/sssilverhand/subforge/main/install.sh | \
  bash -s -- --agent
```

## Конфигурация

| Параметр | Описание |
|----------|----------|
| `server.external_url` | Публичный URL (напр. `https://sub.example.com`) |
| `database.dsn` | Строка подключения к PostgreSQL |
| `auth.jwt_secret` | Секрет для подписи JWT (случайная строка) |
| `auth.super_admin_setup` | Разрешить `/api/setup` (выключить после первого запуска) |
| `bot.enabled` | Включить Telegram бот |
| `bot.payment_mode` | Включить автоматическую оплату |
| `scheduler.traffic_poll_interval` | Интервал опроса трафика (по умолчанию `60s`) |
| `scheduler.expiry_check_interval` | Интервал проверки истечения (по умолчанию `5m`) |

## Управление

```bash
# Статус
systemctl status subforge

# Логи
journalctl -u subforge -f

# Перезапуск
systemctl restart subforge

# Обновление
bash <(curl -Ls https://raw.githubusercontent.com/sssilverhand/subforge/main/install.sh)
```

## Сборка из исходников

Нужны: Go 1.23+, Node.js 18+, PostgreSQL 14+

```bash
git clone https://github.com/sssilverhand/subforge
cd subforge
make build          # собрать для текущей платформы
make build-linux    # кросс-компиляция для Linux amd64 + arm64
```

## Отказ от ответственности

Программное обеспечение предоставляется **«как есть»**, без каких-либо гарантий. Авторы не несут ответственности за любой ущерб, потерю данных, инциденты безопасности или правовые последствия, возникшие в результате использования данного ПО. Используйте на свой страх и риск.

## Лицензия

[GNU Affero General Public License v3.0](LICENSE)  
Форки и производные работы обязаны оставаться открытыми. Коммерческое использование разрешено, но изменения должны публиковаться под той же лицензией.
