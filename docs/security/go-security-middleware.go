package security

import (
    "crypto/rand"
    "encoding/base64"
    "encoding/json"
    "errors"
    "net/http"
    "strings"
    "time"
)

// The browser must never decide paid/VIP/watermark privileges.
type ExportPolicy struct {
    AddWatermark bool
    MaxWidth     int
    AllowSVG     bool
    AllowedFonts map[string]bool
}

type DownloadClaims struct {
    UserID       int64  `json:"uid"`
    GenerationID int64  `json:"gid"`
    Format       string `json:"fmt"`
    Width        int    `json:"w"`
    ExpiresAt    int64  `json:"exp"`
    Nonce        string `json:"nonce"`
}

func NewNonce() (string, error) {
    b := make([]byte, 24)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return base64.RawURLEncoding.EncodeToString(b), nil
}

// SignDownloadToken should use an authenticated scheme such as HMAC-SHA-256
// or an AEAD token, and its nonce must be stored as single-use in Redis.
func BuildClaims(userID, generationID int64, format string, width int) (DownloadClaims, error) {
    if userID <= 0 || generationID <= 0 {
        return DownloadClaims{}, errors.New("invalid identity")
    }
    if format != "png" && format != "svg" && format != "pdf" {
        return DownloadClaims{}, errors.New("unsupported format")
    }
    nonce, err := NewNonce()
    if err != nil {
        return DownloadClaims{}, err
    }
    return DownloadClaims{
        UserID: userID, GenerationID: generationID,
        Format: format, Width: width,
        ExpiresAt: time.Now().Add(2 * time.Minute).Unix(),
        Nonce: nonce,
    }, nil
}

func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
        w.Header().Set("Cache-Control", "no-store")
        next.ServeHTTP(w, r)
    })
}

func DecodeStrictJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
    r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    return dec.Decode(dst)
}

func ClientIP(r *http.Request) string {
    // Trust forwarding headers only when requests come from a configured reverse proxy.
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        return strings.TrimSpace(strings.Split(xff, ",")[0])
    }
    return r.RemoteAddr
}
