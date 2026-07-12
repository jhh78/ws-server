# ws-server

**서버 전용** 실시간 통신 중계기입니다.  
클라이언트 앱은 이 저장소에 포함하지 않습니다. 게임 클라이언트, 채팅 앱, 봇, 관리 도구 등 **외부 프로그램**이 WebSocket으로 접속해 JSON을 주고받습니다.

| | |
|--|--|
| 언어 | Go 1.25+ (`go.mod` 기준) |
| 전송 | TCP + WebSocket (RFC 6455) |
| 메시지 | UTF-8 JSON Text 프레임 |
| 설정 | `sample.env` → `.env` |
| 라이선스 | 루트 `LICENSE` |

---

## 목차

1. [메인 기능·파이프라인](#1-메인-기능파이프라인)
2. [요구 사항·빠른 시작](#2-요구-사항빠른-시작)
3. [설정](#3-설정)
4. [소켓 통신 규격](#4-소켓-통신-규격)
5. [에리어·채널 모델](#5-에리어채널-모델)
6. [에러·제약](#6-에러제약)
7. [확장 훅](#7-확장-훅)
8. [외부 클라이언트 연동 예](#8-외부-클라이언트-연동-예)
9. [테스트](#9-테스트)
10. [디플로이 (Linux)](#10-디플로이-linux)

---

## 1. 메인 기능·파이프라인

서버의 역할은 **수신 → JSON 파싱 → 라우팅/중계 → 응답** 입니다.  
비즈니스 로직(인증·필터 등)은 넣지 않으며, 필요 시 **웹훅**으로 외부에 알립니다.

```
외부 클라이언트 (별도 프로그램)
        │
        │  ws://host:port/ws
        │  Text frame = JSON Envelope
        ▼
┌───────────────────────────────────────────────────────┐
│  client.go   수신 / 송신 펌프, Ping-Pong               │
│       │                                               │
│       ▼                                               │
│  message.go  JSON ↔ Envelope                          │
│       │                                               │
│       ▼                                               │
│  extension.go  로그 + (WEBHOOK_URL 시) 비동기 POST    │
│       │                                               │
│       ▼                                               │
│  route.go    join / leave / send / whisper / ping     │
│       │                                               │
│       ▼                                               │
│  hub.go      에리어·채널 멤버십, 브로드캐스트, 1:1      │
└───────────────────────────────────────────────────────┘
        │  WEBHOOK_URL 설정 시
        ▼
  외부 HTTP 엔드포인트 (JSON POST, best-effort)
```

| 단계 | 파일 | 설명 |
|------|------|------|
| Listen / Upgrade | `server/server.go` | TCP 바인드, HTTP→WS 업그레이드, 헬스 |
| 수신·송신 | `server/client.go` | `onReceive` 파이프라인, write 큐 |
| JSON 규격 | `server/message.go` | `Envelope` |
| 라우팅 | `server/route.go` | type 별 중계·응답 |
| 멤버십 | `server/hub.go` | area / channel 집합 |
| 로그·웹훅 | `server/extension.go`, `webhook.go` | 액세스/시스템 로그, 선택적 이벤트 POST |
| 설정 | `config/` | `.env` 로드 |
| 진입점 | `cmd/ws-server/` | `-env`, `-addr` |

이 저장소에는 UI·게임 클라이언트 소스가 없습니다. 연동은 외부에서 `Envelope` JSON만 맞추면 됩니다.

---

## 2. 요구 사항·빠른 시작

- [Go](https://go.dev/dl/) 1.25 이상
- 방화벽에서 `LISTEN_ADDR` TCP 포트 허용

```bash
git clone https://github.com/jhh78/ws-server.git
cd ws-server
go mod download

cp sample.env .env
go run ./cmd/ws-server/
```

확인:

```bash
curl http://localhost:8080/health
# OK
```

WebSocket URL 예: `ws://localhost:8080/ws`  
(리버스 프록시·TLS 뒤라면 `wss://...`)

### CLI

| 플래그 | 기본 | 설명 |
|--------|------|------|
| `-env` | `.env` | 활성 환경 파일 경로 |
| `-addr` | (없음) | `LISTEN_ADDR` 덮어쓰기 예: `:9090` |

```bash
go run ./cmd/ws-server/ -env .env -addr 0.0.0.0:8080
```

`.env` 가 없으면 기동 실패하며 `cp sample.env .env` 안내가 출력됩니다.

---

## 3. 설정

| 파일 | 역할 |
|------|------|
| `sample.env` | 저장소에 커밋되는 **샘플** |
| `.env` | 런타임 **활성** 설정 (`.gitignore`, 배포 시 생성) |

```bash
cp sample.env .env
# .env 편집 후
./ws-server
```

**우선순위 (높음 → 낮음)**

1. `-addr` (리슨 주소만)
2. 이미 설정된 **OS 환경변수** (파일 값이 덮어쓰지 않음)
3. `-env` 로 지정한 파일 (기본 `.env`)
4. 코드 `config.Default()`

### 환경 변수 전체

| 변수 | 기본 | 설명 |
|------|------|------|
| `SERVER_NAME` | `ws-server` | 로그·`welcome` 표시명 |
| `NETWORK` | `tcp` | `tcp` / `tcp4` / `tcp6` 만 허용 |
| `LISTEN_ADDR` | `:8080` | TCP 바인드 (`0.0.0.0:8080` 등) |
| `WS_PATH` | `/ws` | WebSocket 경로 (`/` 로 시작) |
| `HEALTH_PATH` | `/health` | 헬스 경로 (`WS_PATH` 와 달라야 함) |
| `READ_BUFFER_SIZE` | `4096` | WS 읽기 버퍼(바이트) |
| `WRITE_BUFFER_SIZE` | `4096` | WS 쓰기 버퍼 |
| `ALLOW_ORIGINS` | `*` | 브라우저 Origin. `*` 또는 쉼표 구분 목록 |
| `MAX_CLIENTS_PER_AREA` | `500` | 에리어당 최대 인원 (`0` = 무제한) |
| `MAX_AREAS` | `10000` | 동시 에리어 수 (`0` = 무제한) |
| `MAX_CHANNELS` | `20000` | 동시 채널 수 (`0` = 무제한) |
| `MAX_CLIENTS_PER_CHANNEL` | `200` | 채널당 최대 인원 (`0` = 무제한) |
| `WEBHOOK_URL` | (빈 값) | JSON 배열만. 예 `["https://a/h","https://b/h"]` (비우면 비활성) |
| `WEBHOOK_TIMEOUT_MS` | `5000` | 웹훅 HTTP 타임아웃(ms) |

#### 로그 (시스템 / 액세스)

시스템 로그와 액세스 로그는 **`sample.env` 에 각각 등록**합니다.  
`cp sample.env .env` 후 `.env` 에서 모드·경로·DB 를 조정하세요.

| 구분 | 변수 | 기본 | 설명 |
|------|------|------|------|
| 시스템 | `SYSTEM_LOG_MODE` | `file` | `off` \| `file` \| `db` |
| 시스템 | `SYSTEM_LOG_FILE` | `logs/system.log` | `file` 일 때 경로 |
| 액세스 | `ACCESS_LOG_MODE` | `file` | `off` \| `file` \| `db` |
| 액세스 | `ACCESS_LOG_FILE` | `logs/access.log` | `file` 일 때 경로 |
| DB 공통 | `LOG_DB_DRIVER` | `sqlite` | `sqlite` \| `mysql` \| `postgres` (별칭: sqlite3, mariadb, postgresql, pg) |
| DB 공통 | `LOG_DB_DSN` | `logs/ws-server.db` | 드라이버별 연결 문자열 — **DSN 예는 `sample.env` 주석** |

- **시스템**: 기동, 접속/종료, 업그레이드 실패, listen 오류 등 (`INFO`/`WARN`/`ERROR`)
- **액세스**: `connect` / `disconnect` / `join` / `leave` / `send` / `whisper` / `ping` / `error` / `upgrade_fail`
- 시스템·액세스 모드는 **서로 다르게** 둘 수 있음 (예: 시스템 `file`, 액세스 `db`)
- `db` 모드 시 테이블 `system_logs`, `access_logs` 기동 시 자동 생성. 원격 DB 는 `Ping` 으로 확인

파일 형식: `KEY=VALUE`, `#` 주석, 빈 줄 무시. 알 수 없는 키는 `AppConfig.Extra` 에 보관됩니다 (향후 확장용).

잘못된 정수·비 TCP `NETWORK`·경로 오류·로그 모드 오류는 **기동 시 Load 실패**합니다.

---

## 4. 소켓 통신 규격

### 4.1 전송 계층

| 항목 | 내용 |
|------|------|
| 하위 | TCP (`NETWORK` + `LISTEN_ADDR`) |
| 앱 | WebSocket RFC 6455 |
| 핸드셰이크 | HTTP/1.1 `Upgrade: websocket` |
| 데이터 | **Text** 프레임, UTF-8 JSON |
| 제어 | 서버 주기적 **Ping**, 클라이언트 **Pong** (라이브러리) |
| 최대 페이로드 | 64 KiB (연결당 읽기 한도) |
| 느린 수신자 | 송신 큐(256) 포화 시 해당 프레임 **드롭** (허브 블로킹 방지) |

일반 HTTP GET 으로 `/ws` 를 치면 업그레이드 실패(4xx) — 규격상 정상입니다.

### 4.2 데이터 인터페이스 `Envelope`

모든 비즈니스 메시지는 하나의 JSON 객체입니다.  
Go: `server.Envelope` (`server/message.go`)

```ts
interface Envelope {
  /** 메시지 종류 (필수) */
  type: string;

  /** join / leave / send 범위 */
  scope?: "area" | "channel";

  /** 에리어 ID 또는 채널 ID */
  target?: string;

  /** scope=channel 일 때 채널 종류 */
  channel_kind?: "party" | "guild" | "whisper" | "custom";

  /** whisper 대상 — 서버가 발급한 client_id */
  to?: string;

  /** 표시 이름 (서버가 채우는 경우가 많음) */
  from?: string;

  /** 연결 ID (서버 발급, welcome 에 포함) */
  client_id?: string;

  /** 앱 데이터 — 문자열·객체·배열·숫자 등 임의 JSON */
  payload?: unknown;

  /** type=error 일 때 요약 메시지 */
  error?: string;

  /** 서버 시각 Unix 밀리초 (서버가 채움) */
  ts?: number;
}
```

### 4.3 type 목록

#### 외부 클라이언트 → 서버

| type | 주요 필드 | 서버 동작 |
|------|-----------|-----------|
| `join` | `scope`, `target`, (`channel_kind`) | 멤버십 등록 → `joined` + 타인에게 `system` |
| `leave` | 동일 | 탈퇴 → `left` + 타인 `system` |
| `send` | `scope`, `target`, `payload` | **멤버만** 가능 → 전원에게 `message` (본인 에코 포함) |
| `whisper` | `to`, `payload` | `to` = peer `client_id` → 쌍방 `whisper` |
| `ping` | — | `pong` |

`join` 시 `payload` 로 표시 이름 설정 가능:

- `"Alice"` (JSON 문자열)
- `{ "name": "Alice" }` (객체)

#### 서버 → 외부 클라이언트

| type | 설명 |
|------|------|
| `welcome` | 접속 직후. `client_id`, `server`, `protocol` 안내 |
| `joined` | 입장 성공 확인 |
| `left` | 퇴장 확인 |
| `message` | `send` 결과 브로드캐스트 |
| `whisper` | 1:1 |
| `system` | 타 유저 join/leave 등 (`payload.event`) |
| `error` | 실패 (`error` 필드 + `payload.message`) |
| `pong` | `ping` 응답 |

`welcome.payload.protocol` 예:

```json
{
  "version": 1,
  "encoding": "json",
  "transport": "websocket",
  "types": ["join", "leave", "send", "whisper", "ping"],
  "scopes": ["area", "channel"],
  "channel_kinds": ["party", "guild", "whisper", "custom"]
}
```

### 4.4 시퀀스 예

**에리어**

```json
→ {"type":"join","scope":"area","target":"map-1","payload":{"name":"Alice"}}
← {"type":"joined","scope":"area","target":"map-1","client_id":"c1","from":"Alice",...}

→ {"type":"send","scope":"area","target":"map-1","payload":{"text":"hi","x":1}}
← {"type":"message","scope":"area","target":"map-1","from":"Alice","client_id":"c1",
    "payload":{"text":"hi","x":1},"ts":...}
```

**채널 (파티)**

```json
→ {"type":"join","scope":"channel","channel_kind":"party","target":"party-42","payload":{"name":"Alice"}}
→ {"type":"send","scope":"channel","channel_kind":"party","target":"party-42","payload":{"text":"ready?"}}
← {"type":"message","scope":"channel","channel_kind":"party","target":"party-42",...}
```

**귓속말**

```json
→ {"type":"whisper","to":"c2","payload":{"text":"secret"}}
← {"type":"whisper","from":"Alice","client_id":"c1","to":"c2","payload":{"text":"secret"}}
```

---

## 5. 에리어·채널 모델

서버는 두 종류의 **구독 범위**를 동시에 지원합니다. 한 연결은 여러 에리어·채널에 동시에 들어갈 수 있습니다.

| 개념 | `scope` | 식별 | 용도 예 |
|------|---------|------|---------|
| 에리어 | `area` | `target` = 에리어 ID | 맵, 존, 로비, 인스턴스 |
| 채널 | `channel` | `channel_kind` + `target` | 파티, 길드, 귓속말 방, 커스텀 방 |

내부 채널 키: `"{channel_kind}:{target}"` 예: `party:party-42`

| `channel_kind` | 의미 |
|----------------|------|
| `party` | 파티 |
| `guild` | 길드 |
| `whisper` | 귓속말용 룸(선택) — 1:1 은 보통 `type=whisper` 사용 |
| `custom` | 기타 (`channel_kind` 생략 시 기본값) |

**전달 규칙**

1. **에리어 `send`**: 해당 `target` 에 `join` 한 클라이언트만 `message` 수신. 미가입자는 수신·발신 모두 불가(발신은 `error`).
2. **채널 `send`**: 동일 kind+target 멤버만 수신.
3. **`whisper`**: `to` 로 지정한 `client_id` 한 명 + 발신자 에코. 제3자 없음.
4. **연결 종료**: 해당 클라이언트의 모든 에리어·채널 멤버십 자동 해제.

---

## 6. 에러·제약

| 상황 | 응답 |
|------|------|
| JSON 파싱 실패 | `type=error`, `error=invalid json message` |
| 알 수 없는 `type` | `unknown type: ...` |
| `scope` 오류 | `scope must be area or channel` |
| 미가입 `send` | `not a member of area/channel; join first` |
| 에리어/채널 가득 참 | `area is full` / `channel is full` |
| 한도 초과 생성 | `max areas reached` / `max channels reached` |
| whisper `to` 없음/오프라인 | `to (client_id) is required` / `peer not found` |

`error` 필드와 `payload.message` 에 동일 요약이 들어갑니다.

기타:

- 유효하지 않은 JSON 키 조합은 라우팅 단계에서 거부됩니다.
- 인바운드/아웃바운드 훅은 **중계 전용 통과**이며 메시지 드롭·변조를 하지 않습니다.

---

## 7. 웹훅 (선택)

이 서버는 **중계 전용**입니다. 외부 백엔드가 이벤트를 받으려면 `sample.env` 의 `WEBHOOK_URL` 을 설정하세요.

| 변수 | 설명 |
|------|------|
| `WEBHOOK_URL` | JSON 배열 문자열만. 비우거나 `[]` 이면 끔 |
| `WEBHOOK_TIMEOUT_MS` | POST 타임아웃 (기본 5000) |

**`WEBHOOK_URL` 형식 (통일)**

```env
WEBHOOK_URL=["https://a.example/hook","https://b.example/hook"]
WEBHOOK_URL=["https://one.example/hook"]
WEBHOOK_URL=
```

쉼표 나열·따옴표 없는 단일 URL 은 허용하지 않습니다. 파싱: `config.ParseWebhookURLs`.

**요청:** `POST` · `Content-Type: application/json; charset=utf-8` · `User-Agent: ws-server-webhook/1`  
**POST 시점:** `connect`, `disconnect`, `join`, `leave`, `send`, `whisper`, `ping`  
(비동기 — 실패해도 WS 중계 계속)

**공통 스키마 (`WebhookPayload`)**

| 필드 | 타입 | 설명 |
|------|------|------|
| `event` | string | 이벤트 이름 |
| `ts` | number | Unix 밀리초 |
| `server` | string | `SERVER_NAME` |
| `client_id` | string | 서버 발급 연결 ID |
| `remote_addr` | string | 피어 주소 |
| `envelope` | object? | 인바운드 `Envelope` 스냅샷. `connect`/`disconnect` 에는 없음 |

코드 상수: `server/webhook.go` 의 `WebhookSample*` (런타임 미사용, 문서·수신 측 참고용).

### 7.1 샘플 — connect

```json
{
  "event": "connect",
  "ts": 1710000000000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321"
}
```

### 7.2 샘플 — disconnect

```json
{
  "event": "disconnect",
  "ts": 1710000005000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321"
}
```

### 7.3 샘플 — join (area)

```json
{
  "event": "join",
  "ts": 1710000001000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "join",
    "scope": "area",
    "target": "lobby",
    "payload": { "name": "Alice" }
  }
}
```

### 7.4 샘플 — join (channel / party)

```json
{
  "event": "join",
  "ts": 1710000001100,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "join",
    "scope": "channel",
    "channel_kind": "party",
    "target": "party-42",
    "payload": { "name": "Alice" }
  }
}
```

### 7.5 샘플 — leave

```json
{
  "event": "leave",
  "ts": 1710000002000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "leave",
    "scope": "area",
    "target": "lobby"
  }
}
```

### 7.6 샘플 — send

```json
{
  "event": "send",
  "ts": 1710000003000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "send",
    "scope": "area",
    "target": "lobby",
    "payload": { "text": "hello", "x": 1, "y": 2 }
  }
}
```

### 7.7 샘플 — whisper

```json
{
  "event": "whisper",
  "ts": 1710000004000,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "whisper",
    "to": "c2",
    "payload": { "text": "secret" }
  }
}
```

### 7.8 샘플 — ping

```json
{
  "event": "ping",
  "ts": 1710000004500,
  "server": "ws-server",
  "client_id": "c1",
  "remote_addr": "203.0.113.10:54321",
  "envelope": {
    "type": "ping"
  }
}
```

| 코드 상수 | 이벤트 |
|-----------|--------|
| `WebhookSampleConnect` | connect |
| `WebhookSampleDisconnect` | disconnect |
| `WebhookSampleJoinArea` | join (area) |
| `WebhookSampleJoinChannel` | join (channel) |
| `WebhookSampleLeave` | leave |
| `WebhookSampleSend` | send |
| `WebhookSampleWhisper` | whisper |
| `WebhookSamplePing` | ping |

- 구현: `server/webhook.go`, 호출: `server/extension.go`
- 인증·저장·푸시 등 **비즈니스는 웹훅 수신 측**에서 처리

---

## 8. 외부 클라이언트 연동 예

서버 저장소 밖 코드 예시입니다 (참고용). 공통 흐름:

1. WebSocket 연결 (`ws://` 또는 프록시 뒤 `wss://`)
2. `welcome` 에서 `client_id` 저장 (귓속말용)
3. `join` 후 `send` / `whisper`
4. 수신 `type` 분기 처리

| 환경 | URL 예 |
|------|--------|
| 직접 소켓 | `ws://127.0.0.1:8080/ws` |
| 리버스 프록시 + TLS | `wss://api.example.com/ws` |

서버가 하지 않는 일: 로그인 UI, 푸시 알림, 영구 채팅 저장(확장으로 구현).

### 8.1 JavaScript (브라우저 / Node)

```javascript
// 브라우저: WebSocket 내장
// Node: npm i ws  후  const WebSocket = require("ws");

const url = "ws://127.0.0.1:8080/ws";
const ws = new WebSocket(url);
let clientId = null;

ws.onopen = () => {
  console.log("connected");
};

ws.onmessage = (ev) => {
  const msg = JSON.parse(typeof ev.data === "string" ? ev.data : ev.data.toString());
  console.log("←", msg);

  if (msg.type === "welcome") {
    clientId = msg.client_id;
    ws.send(JSON.stringify({
      type: "join",
      scope: "area",
      target: "lobby",
      payload: { name: "js-user" },
    }));
  }

  if (msg.type === "joined") {
    ws.send(JSON.stringify({
      type: "send",
      scope: "area",
      target: "lobby",
      payload: { text: "hello from js" },
    }));
  }
};

ws.onerror = (e) => console.error(e);
ws.onclose = () => console.log("closed");
```

### 8.2 Flutter (Dart)

```dart
// pubspec.yaml:  web_socket_channel: ^3.0.0

import 'dart:convert';
import 'package:web_socket_channel/web_socket_channel.dart';

void main() async {
  final uri = Uri.parse('ws://127.0.0.1:8080/ws');
  final channel = WebSocketChannel.connect(uri);
  String? clientId;

  channel.stream.listen((raw) {
    final msg = jsonDecode(raw as String) as Map<String, dynamic>;
    print('← $msg');

    if (msg['type'] == 'welcome') {
      clientId = msg['client_id'] as String?;
      channel.sink.add(jsonEncode({
        'type': 'join',
        'scope': 'area',
        'target': 'lobby',
        'payload': {'name': 'flutter-user'},
      }));
    }

    if (msg['type'] == 'joined') {
      channel.sink.add(jsonEncode({
        'type': 'send',
        'scope': 'area',
        'target': 'lobby',
        'payload': {'text': 'hello from flutter'},
      }));
    }
  });
}
```

### 8.3 PHP (Ratchet / ReactPHP 스타일 클라이언트)

```php
<?php
// composer require textalk/websocket

use WebSocket\Client;

$url = 'ws://127.0.0.1:8080/ws';
$client = new Client($url);
$clientId = null;

// 서버 welcome 수신
$raw = $client->receive();
$msg = json_decode($raw, true);
if (($msg['type'] ?? '') === 'welcome') {
    $clientId = $msg['client_id'] ?? null;
    $client->send(json_encode([
        'type'    => 'join',
        'scope'   => 'area',
        'target'  => 'lobby',
        'payload' => ['name' => 'php-user'],
    ]));
}

// joined 대기 후 send
$raw = $client->receive();
$msg = json_decode($raw, true);
if (($msg['type'] ?? '') === 'joined') {
    $client->send(json_encode([
        'type'    => 'send',
        'scope'   => 'area',
        'target'  => 'lobby',
        'payload' => ['text' => 'hello from php'],
    ]));
}

// 이후 메시지 수신 루프 예
// while (true) { echo $client->receive(), PHP_EOL; }

$client->close();
```

### 8.4 C# (.NET)

```csharp
// Package: System.Net.WebSockets (내장) 또는 ClientWebSocket

using System.Net.WebSockets;
using System.Text;
using System.Text.Json;

var url = new Uri("ws://127.0.0.1:8080/ws");
using var ws = new ClientWebSocket();
await ws.ConnectAsync(url, CancellationToken.None);

async Task SendAsync(object body)
{
    var json = JsonSerializer.Serialize(body);
    var bytes = Encoding.UTF8.GetBytes(json);
    await ws.SendAsync(bytes, WebSocketMessageType.Text, true, CancellationToken.None);
}

async Task<JsonElement> ReceiveAsync()
{
    var buf = new byte[64 * 1024];
    var result = await ws.ReceiveAsync(buf, CancellationToken.None);
    var json = Encoding.UTF8.GetString(buf, 0, result.Count);
    Console.WriteLine("← " + json);
    return JsonDocument.Parse(json).RootElement;
}

var welcome = await ReceiveAsync();
if (welcome.GetProperty("type").GetString() == "welcome")
{
    var clientId = welcome.GetProperty("client_id").GetString();
    await SendAsync(new
    {
        type = "join",
        scope = "area",
        target = "lobby",
        payload = new { name = "csharp-user" }
    });
}

var joined = await ReceiveAsync();
if (joined.GetProperty("type").GetString() == "joined")
{
    await SendAsync(new
    {
        type = "send",
        scope = "area",
        target = "lobby",
        payload = new { text = "hello from csharp" }
    });
}

await ws.CloseAsync(WebSocketCloseStatus.NormalClosure, "bye", CancellationToken.None);
```

### 8.5 Java (Java-WebSocket)

```java
// Maven: org.java-websocket:Java-WebSocket:1.5.6

import org.java_websocket.client.WebSocketClient;
import org.java_websocket.handshake.ServerHandshake;
import org.json.JSONObject;

import java.net.URI;

public class WsClientExample {
    public static void main(String[] args) throws Exception {
        URI uri = new URI("ws://127.0.0.1:8080/ws");

        WebSocketClient client = new WebSocketClient(uri) {
            @Override
            public void onOpen(ServerHandshake handshakedata) {
                System.out.println("connected");
            }

            @Override
            public void onMessage(String message) {
                System.out.println("← " + message);
                JSONObject msg = new JSONObject(message);
                String type = msg.optString("type");

                if ("welcome".equals(type)) {
                    String clientId = msg.optString("client_id");
                    JSONObject join = new JSONObject()
                        .put("type", "join")
                        .put("scope", "area")
                        .put("target", "lobby")
                        .put("payload", new JSONObject().put("name", "java-user"));
                    send(join.toString());
                }

                if ("joined".equals(type)) {
                    JSONObject sendMsg = new JSONObject()
                        .put("type", "send")
                        .put("scope", "area")
                        .put("target", "lobby")
                        .put("payload", new JSONObject().put("text", "hello from java"));
                    send(sendMsg.toString());
                }
            }

            @Override
            public void onClose(int code, String reason, boolean remote) {
                System.out.println("closed: " + reason);
            }

            @Override
            public void onError(Exception ex) {
                ex.printStackTrace();
            }
        };

        client.connectBlocking();
        Thread.sleep(5000);
        client.close();
    }
}
```

---

## 9. 테스트

테스트 코드는 모두 **`test/`** (서버 규격·중계 검증용; 제품 클라이언트 아님).

```bash
go test ./...
go test -v ./test/
go test -v ./test/ -run 'TestArea|TestChannel|TestWhisper'
```

| 테스트 (요지) | 검증 |
|---------------|------|
| `TestAreaBroadcastToAllMembers` | 에리어 멤버 전원 수신, 외부자 미수신 |
| `TestChannelBroadcastParty` | 파티 채널 격리 |
| `TestChannelGuild` | 길드 채널 |
| `TestWhisperToClient` | 1:1 귓속말 |
| `TestSendRequiresMembership` | 미가입 send 거부 |
| `TestLoadSampleEnv` | `sample.env` 로드 |

---

## 10. 디플로이 (Linux)

### 빌드

```bash
GOOS=linux GOARCH=amd64 go build -o ws-server ./cmd/ws-server/
GOOS=linux GOARCH=arm64 go build -o ws-server-arm64 ./cmd/ws-server/
```

### 배포

```bash
scp ws-server sample.env user@host:/opt/ws-server/
ssh user@host
cd /opt/ws-server
chmod +x ws-server
cp sample.env .env
# .env 의 LISTEN_ADDR, ALLOW_ORIGINS 등 수정
./ws-server
```

클라이언트 접속 방식은 아래 둘 중 하나를 선택합니다.

### 직접 소켓 통신

클라이언트가 `ws-server` 에 **바로** 연결합니다.

| 항목 | 내용 |
|------|------|
| URL | `ws://<서버IP 또는 호스트>:8080/ws` (`LISTEN_ADDR`·`WS_PATH` 에 맞춤) |
| 방화벽 | `LISTEN_ADDR` TCP 포트 인바운드 허용 |
| TLS | 앱 자체는 plain WS. 필요 시 앞단 프록시 사용(아래) |
| 용도 | 사내망, 게임 클라이언트 직접 접속, 개발/테스트 |

```bash
# 예: 모든 인터페이스에서 수신
# .env → LISTEN_ADDR=0.0.0.0:8080
./ws-server
```

### 리버스 프록시 경유

Nginx, Caddy, 클라우드 LB 등으로 TLS 종료·도메인 노출 후, 백엔드는 내부 주소로만 띄웁니다.

| 항목 | 내용 |
|------|------|
| 클라이언트 URL | `wss://api.example.com/ws` |
| 백엔드 | `http://127.0.0.1:8080` (또는 private IP) |
| 필수 헤더 | `Upgrade`, `Connection: upgrade` (WebSocket 유지) |
| 타임아웃 | 장시간 연결이면 read/idle timeout 을 충분히 크게 |

**Nginx 예**

```nginx
location /ws {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

**Caddy 예**

```caddy
api.example.com {
    reverse_proxy /ws* 127.0.0.1:8080
}
```

정리: **직접 소켓**은 단순·저지연, **프록시**는 TLS·도메인·접근 제어에 유리합니다. 프로토콜(JSON Envelope)은 동일합니다.

---

## 라이선스

저장소 루트 `LICENSE` 파일을 참고하세요.
