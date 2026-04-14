package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/cuducos/go-cnpj"
)

const (
	retries      = 3
	queryTimeout = 3 * time.Second
)

func getCompany(ctx context.Context, db database, n string) (string, error) {
	var c string
	err := retry.Do(
		func() error {
			ctx, cancel := context.WithTimeout(ctx, queryTimeout)
			defer cancel()
			var err error
			c, err = db.GetCompany(ctx, cnpj.Unmask(n))
			return err
		},
		retry.Attempts(retries),
		retry.RetryIf(func(err error) bool {
			return errors.Is(err, context.DeadlineExceeded)
		}),
		retry.Context(ctx),
	)
	if err != nil {
		return "", fmt.Errorf("error retrieving %s: %w", n, err)
	}
	return c, nil
}
