package app

import "github.com/yeisme/pinax/internal/domain"

// commandErrorProjection is shared by application services that need to return
// both a machine-readable projection and the original error. It used to live
// beside the removed KB service; keep the generic helper in the app package so
// sync and daemon failures retain their stable output contract.
func commandErrorProjection(command string, err error) (domain.Projection, error) {
	if cmdErr, ok := err.(*domain.CommandError); ok {
		return domain.NewErrorProjection(command, cmdErr), cmdErr
	}
	return errorProjection(command, err), err
}
