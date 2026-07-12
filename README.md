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
11. [프로젝트 구조](#11-프로젝트-구조)

---

## 1. 메인 기능·파이프라인

서버의 역할은 **수신 → JSON 처리 → (확장) → 라우팅/중계 → 응답** 입니다.

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
│  extension.go  빈 훅 (인증·필터·로그 등 확장 지점)      │
│       │                                               │
│       ▼                                               │
│  route.go    join / leave / send / whisper / ping     │
│       │                                               │
│       ▼                                               │
│  hub.go      에리어·채널 멤버십, 브로드캐스트, 1:1      │
└───────────────────────────────────────────────────────┘
```

| 단계 | 파일 | 설명 |
|------|------|------|
| Listen / Upgrade | `server/server.go` | TCP 바인드, HTTP→WS 업그레이드, 헬스 |
| 수신·송신 | `server/client.go` | `onReceive` 파이프라인, write 큐 |
| JSON 규격 | `server/message.go` | `Envelope` |
| 라우팅 | `server/route.go` | type 별 처리·응답 |
| 멤버십 | `server/hub.go` | area / channel 집합 |
| **확장** | `server/extension.go` | **빈 함수** — 비즈니스 로직 연결점 |
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

파일 형식: `KEY=VALUE`, `#` 주석, 빈 줄 무시. 알 수 없는 키는 `AppConfig.Extra` 에 보관됩니다 (향후 확장용).

잘못된 정수·비 TCP `NETWORK`·경로 오류는 **기동 시 Load 실패**합니다.

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
- `extProcessInbound` / `extProcessOutbound` 가 `drop=true` 를 반환하면 **무응답 드롭** 가능(확장 구현 시 주의).

---

## 7. 확장 훅

파일: **`server/extension.go`**

메인 루프는 고정이고, 부가 처리는 훅 본문만 채우면 됩니다.  
같은 `package server` 의 새 파일(예: `auth.go`, `filter.go`)에 로직을 두고 훅에서 호출하는 방식을 권장합니다.

| 함수 | 호출 시점 | 용도 예 |
|------|-----------|---------|
| `extOnConnect` | Hub 등록 직후, `welcome` 전 | 접속 로그, 세션 시작 |
| `extOnDisconnect` | 종료 처리 직전 | 세션 정리, 오프라인 알림 |
| `extProcessInbound` | JSON 파싱 후, 라우팅 전 | 권한, payload 변환, 커스텀 type |
| `extProcessOutbound` | 송신 큐 직전 | 필터, 검열, 감사 로그 |
| `extOnRouted` | join/send 등 라우팅 완료 후 | 메트릭, 비동기 후처리 |

시그니처 요지:

```go
func extProcessInbound(c *Client, msg *Envelope) (out *Envelope, drop bool)
func extProcessOutbound(c *Client, msg *Envelope) (out *Envelope, drop bool)
```

- `drop == true`: 이후 단계 생략 (필요 시 훅 안에서 `c.reply` 로 에러 전송 가능 — `reply` 는 package 내부 API)
- `out` 으로 수정된 `Envelope` 전달 가능
- 기본 구현: **통과** (`return msg, false`)

---

## 8. 외부 클라이언트 연동 예

서버 저장소 밖 코드 예시입니다 (참고용).

**브라우저 콘솔**

```javascript
const ws = new WebSocket("ws://localhost:8080/ws");
ws.onmessage = (e) => console.log(JSON.parse(e.data));
ws.onopen = () => {
  ws.send(JSON.stringify({
    type: "join", scope: "area", target: "lobby",
    payload: { name: "web-user" }
  }));
};
```

**접속 URL**

| 환경 | URL |
|------|-----|
| 로컬 | `ws://127.0.0.1:8080/ws` |
| TLS 리버스 프록시 | `wss://api.example.com/ws` |

클라이언트가 해야 할 일:

1. WebSocket 연결
2. `welcome` 에서 `client_id` 저장 (귓속말용)
3. 필요 시 `join` 후 `send` / `whisper`
4. 수신 `type` 분기 처리

서버가 하지 않는 일: 로그인 UI, 푸시 알림(Firebase 등 별도), 영구 채팅 저장(확장으로 구현).

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

방화벽에서 **TCP** 포트(기본 8080) 인바운드를 허용합니다.

### systemd 예

`/etc/systemd/system/ws-server.service`:

```ini
[Unit]
Description=ws-server (WebSocket over TCP)
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/ws-server
ExecStart=/opt/ws-server/ws-server
Restart=on-failure
RestartSec=5
# Environment=LISTEN_ADDR=:8080

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ws-server
sudo systemctl status ws-server
```

### Nginx (WebSocket 프록시)

```nginx
location /ws {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_read_timeout 3600s;
}
```

TLS 는 Nginx(또는 로드밸런서)에서 종료하고, 앱은 내부 HTTP/WS 로 두는 구성을 권장합니다.

---

## 11. 프로젝트 구조

```
ws-server/                    # 서버 전용 저장소
├── sample.env                # 설정 샘플 → cp sample.env .env
├── cmd/ws-server/main.go     # 진입점 (-env, -addr)
├── config/
│   ├── config.go             # AppConfig, Load, Validate
│   └── load.go               # .env 파서
├── server/
│   ├── server.go             # TCP Listen, Upgrade, 헬스
│   ├── client.go             # 수신·송신, onReceive 파이프라인
│   ├── route.go              # type 라우팅·응답
│   ├── hub.go                # 에리어·채널 멤버십
│   ├── message.go            # Envelope JSON
│   └── extension.go          # 확장 빈 함수 ★
├── test/                     # 중계·규격 테스트
├── go.mod
├── README.md
└── LICENSE
```

---

## 라이선스

저장소 루트 `LICENSE` 파일을 참고하세요.
