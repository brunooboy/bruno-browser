//go:build !windows

package updates

import (
	"context"
	"errors"
)

func (s *Service) LaunchApplyHelper(context.Context, DownloadResult) error {
	return errors.New("a atualização automática está disponível somente no Windows")
}

func RunApplyHelper([]string) (bool, error) { return false, nil }
func UpdateHealthMarker([]string) string    { return "" }
func MarkHealthy(string, string) error      { return nil }
