package http

// Cookie names are no longer needed for session management since
// exe.dev handles authentication via proxy headers.
//
// The only cookies remaining are for the SPA to pass transient state
// (e.g., return-after-login URLs), but those are handled by exe.dev
// /__exe.dev/login?redirect=... natively.
