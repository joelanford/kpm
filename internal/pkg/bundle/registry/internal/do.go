package internal

import "errors"

func DoAll(funcs ...func() error) error {
	var errs []error
	for _, fn := range funcs {
		errs = append(errs, fn())
	}
	return errors.Join(errs...)
}
