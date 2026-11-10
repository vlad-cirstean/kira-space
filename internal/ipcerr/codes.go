package ipcerr

// Code is a wire error code. The TS twin is packages/shared/domain/errorCodes.ts.
type Code string

// Generic codes.
const (
	CodeInternal     Code = "E_INTERNAL"
	CodeContract     Code = "E_CONTRACT"
	CodeBadRequest   Code = "E_BAD_REQUEST"
	CodeNotFound     Code = "E_NOT_FOUND"
	CodeConflict     Code = "E_CONFLICT"
	CodeUnavailable  Code = "E_UNAVAILABLE"
	CodeTimeout      Code = "E_TIMEOUT"
	CodeCancelled    Code = "E_CANCELLED"
	CodeTransport    Code = "E_TRANSPORT"
	CodeUnauthorized Code = "E_UNAUTHORIZED"
	CodeForbidden    Code = "E_FORBIDDEN"
	CodeReadOnly     Code = "E_READ_ONLY"
	CodeTooLarge     Code = "E_TOO_LARGE"
	CodeUnsupported  Code = "E_UNSUPPORTED"
	CodeRateLimited  Code = "E_RATE_LIMITED"
	CodeDisconnected Code = "E_DISCONNECTED"
	CodeSecretStore  Code = "E_SECRET_STORE"
)

// Domain codes: kept only where a UI branches on them.
const (
	CodeConnect             Code = "E_CONNECT"
	CodeAuth                Code = "E_AUTH"
	CodeQuery               Code = "E_QUERY"
	CodeEngineDown          Code = "E_ENGINE_DOWN"
	CodeGitNotARepository   Code = "E_GIT_NOT_A_REPOSITORY"
	CodeGitPermissionDenied Code = "E_GIT_PERMISSION_DENIED"
	CodeGitCancelled        Code = "E_GIT_CANCELLED"
	CodeGitTimeout          Code = "E_GIT_TIMEOUT"
	CodeGitUnknown          Code = "E_GIT_UNKNOWN"
	CodeGitUnavailable      Code = "E_GIT_UNAVAILABLE"
	CodePreparing           Code = "E_PREPARING"
	CodeTaskNoRepo          Code = "E_TASK_NO_REPO"
	CodeRevoked             Code = "E_REVOKED"
	CodeExpired             Code = "E_EXPIRED"
	CodePairingDenied       Code = "E_PAIRING_DENIED"
	CodeWriteOff            Code = "E_WRITE_OFF"
	CodeAgentInputOff       Code = "E_AGENT_INPUT_OFF"
	CodeRecreateFailed      Code = "E_RECREATE_FAILED"
	CodeAlreadyImported     Code = "E_ALREADY_IMPORTED"
	CodeUnknownMethod       Code = "E_UNKNOWN_METHOD"
	CodeIdempotencyKey      Code = "E_IDEMPOTENCY_KEY"
	CodeIdempotencyMismatch Code = "E_IDEMPOTENCY_MISMATCH"
	CodeFrameTooLarge       Code = "E_FRAME_TOO_LARGE"
)
