package utils

// UniqueErrors filters out the duplicates in the input slice of errors
// and returns a single error when the error strings are joined by comma
// character
func UniqueErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	m := make(map[string]struct{})
	uniqueErrs := make([]error, 0)

	for _, err := range errs {
		if _, ok := m[err.Error()]; !ok {
			m[err.Error()] = struct{}{}
			uniqueErrs = append(uniqueErrs, err)
		}
	}

	return &joinError{errs: uniqueErrs}
}

type joinError struct {
	errs []error
}

func (j *joinError) Error() string {
	if len(j.errs) == 1 {
		return j.errs[0].Error()
	}
	s := j.errs[0].Error()
	for _, e := range j.errs[1:] {
		s = s + ", " + e.Error()
	}
	return s
}

func (j *joinError) Unwrap() []error {
	return j.errs
}

// ErrorsToStrings turns error slice to string slice
func ErrorsToStrings(errs []error) []string {
	var result []string
	for _, err := range errs {
		result = append(result, err.Error())
	}

	return result
}
