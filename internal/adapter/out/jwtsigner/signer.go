// Package jwtsigner mintea JWTs propios firmados con la clave privada de
// einar y publica la clave pública en formato JWKS.
//
// Estos JWTs son DIFERENTES del id_token de Casdoor (que se usa para la
// sesión del shell). Sirven para que el backend de un developer tercero
// valide requests venidos desde su iframe embebido sin tener que
// integrar Casdoor: el dev valida el JWT contra
//   https://einar.exe.xyz/.well-known/einar/jwks
// y obtiene claims ricos: sub, email, tenant_id, tenant_slug, role.
//
// Decisión de claves:
//   - Si EINAR_JWT_PRIVATE_KEY (PEM RSA) está seteada → la usamos.
//   - Si no → generamos un par ephemeral al startup y loggeamos warning.
//     En producción esto rota la clave en cada restart e invalida tokens
//     en vuelo; aceptable para dev, NO para prod.
package jwtsigner

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"time"

	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/golang-jwt/jwt/v5"
)

const (
	// Algoritmo de firma. RS256 es el estándar para JWKS y soportado
	// por todas las libs (jose, go-oidc, jsonwebtoken, python-jose, etc).
	signingAlg = "RS256"

	// kid (key id) determinístico derivado del modulus de la pública.
	// Si rotás la clave, cambia automáticamente.
	defaultKid = "einar-rsa-1"
)

type Signer struct {
	priv   *rsa.PrivateKey
	pub    *rsa.PublicKey
	kid    string
	issuer string
}

var _ = ioc.Register(NewSigner)

func NewSigner(cfg environment.Conf) (*Signer, error) {
	priv, err := loadOrGenerate(cfg)
	if err != nil {
		return nil, err
	}

	// kid derivado de SHA-256 del modulus → estable mientras la clave
	// no rote, distinto entre claves diferentes.
	hash := sha256.Sum256(priv.PublicKey.N.Bytes())
	kid := defaultKid + "-" + base64.RawURLEncoding.EncodeToString(hash[:8])

	// El issuer del JWT es la URL pública del shell.
	issuer := cfg.APP_PUBLIC_URL
	if issuer == "" {
		issuer = "https://einar.exe.xyz"
	}

	return &Signer{
		priv:   priv,
		pub:    &priv.PublicKey,
		kid:    kid,
		issuer: issuer,
	}, nil
}

func loadOrGenerate(cfg environment.Conf) (*rsa.PrivateKey, error) {
	if pemStr := cfg.EINAR_JWT_PRIVATE_KEY; pemStr != "" {
		// Soportar tanto PEM directo como un path a archivo.
		if _, err := os.Stat(pemStr); err == nil {
			b, err := os.ReadFile(pemStr)
			if err != nil {
				return nil, fmt.Errorf("read EINAR_JWT_PRIVATE_KEY file: %w", err)
			}
			pemStr = string(b)
		}
		block, _ := pem.Decode([]byte(pemStr))
		if block == nil {
			return nil, errors.New("EINAR_JWT_PRIVATE_KEY no es un PEM válido")
		}
		// Soportar PKCS1 ("RSA PRIVATE KEY") y PKCS8 ("PRIVATE KEY").
		if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			slog.Info("einar JWT signer: clave RSA cargada desde env (PKCS1)")
			return k, nil
		}
		if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			rsaK, ok := k.(*rsa.PrivateKey)
			if !ok {
				return nil, errors.New("EINAR_JWT_PRIVATE_KEY no es RSA")
			}
			slog.Info("einar JWT signer: clave RSA cargada desde env (PKCS8)")
			return rsaK, nil
		}
		return nil, errors.New("no pude parsear EINAR_JWT_PRIVATE_KEY (probé PKCS1 y PKCS8)")
	}

	// Sin env var: generar ephemeral. OK para dev. Warning fuerte.
	slog.Warn("einar JWT signer: generando clave RSA ephemeral; " +
		"los embedded tokens NO sobreviven al restart del proceso. " +
		"Para producción seteá EINAR_JWT_PRIVATE_KEY (PEM PKCS8 RSA 2048+).")
	return rsa.GenerateKey(rand.Reader, 2048)
}

// Mint genera un JWT firmado con la clave privada de einar.
//
// `audience` típicamente es el origin de la app embedded (ej:
// "https://app-developer.com"). El backend del dev configura su
// validador con la misma audience para evitar token reuse cruzado
// entre apps embedded.
//
// `ttl` es lo que vive el token. Recomendado: 5-15 minutos. El SDK
// del shell hace refresh proactivo antes de expirar.
type Claims struct {
	Email       string `json:"email,omitempty"`
	Name        string `json:"name,omitempty"`
	TenantID    string `json:"tenant_id,omitempty"`
	TenantSlug  string `json:"tenant_slug,omitempty"`
	Role        string `json:"role,omitempty"`
	jwt.RegisteredClaims
}

func (s *Signer) Mint(c Claims, audience string, ttl time.Duration) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(ttl)

	c.Issuer = s.issuer
	c.IssuedAt = jwt.NewNumericDate(now)
	c.NotBefore = jwt.NewNumericDate(now)
	c.ExpiresAt = jwt.NewNumericDate(exp)
	if audience != "" {
		c.Audience = jwt.ClaimStrings{audience}
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	tok.Header["kid"] = s.kid
	tok.Header["typ"] = "JWT"

	signed, err := tok.SignedString(s.priv)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// Verify parsea y valida un JWT minteado por este Signer.
//
// Útil para /api/whoami y otros endpoints que aceptan tokens de
// embedded apps. El audience es opcional: si lo pasás vacío, no se
// chequea (útil para introspection genérica).
func (s *Signer) Verify(raw string, expectedAudience string) (*Claims, error) {
	c := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, c, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != signingAlg {
			return nil, fmt.Errorf("alg unexpected: %s", t.Method.Alg())
		}
		if kid, _ := t.Header["kid"].(string); kid != s.kid {
			return nil, fmt.Errorf("kid unknown: %s", kid)
		}
		return s.pub, nil
	}, jwt.WithIssuer(s.issuer))
	if err != nil {
		return nil, err
	}
	if !tok.Valid {
		return nil, errors.New("token invalid")
	}
	if expectedAudience != "" {
		ok := false
		for _, a := range c.Audience {
			if a == expectedAudience {
				ok = true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("audience mismatch: %v", c.Audience)
		}
	}
	return c, nil
}

// JWKS retorna la representación JSON Web Key Set de la clave pública.
// Esto es lo que sirve el endpoint /.well-known/einar/jwks.
func (s *Signer) JWKS() map[string]any {
	n := base64.RawURLEncoding.EncodeToString(s.pub.N.Bytes())
	// e siempre 65537 → 0x010001 → "AQAB"
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(s.pub.E)).Bytes())
	return map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"alg": signingAlg,
			"use": "sig",
			"kid": s.kid,
			"n":   n,
			"e":   e,
		}},
	}
}

// Issuer expone el issuer configurado (para discovery doc, /api/whoami).
func (s *Signer) Issuer() string { return s.issuer }
