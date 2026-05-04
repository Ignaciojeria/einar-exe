package environment

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleEnvLoad(t *testing.T) {
	handleEnvLoad(nil)
	handleEnvLoad(errors.New("file not found"))
}

func TestParse(t *testing.T) {
	type Config struct {
		Port int    `env:"TEST_PARSE_PORT" envDefault:"8080"`
		Host string `env:"TEST_PARSE_HOST" envDefault:"localhost"`
	}
	conf, err := Parse[Config]()
	require.NoError(t, err)
	assert.Equal(t, 8080, conf.Port)
	assert.Equal(t, "localhost", conf.Host)
}