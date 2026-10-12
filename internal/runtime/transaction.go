package runtime

// ApplyPolicy checkpoints the inverse plan before each mutation. The caller
// owns rollback so it can retain the journal after ambiguous timeout failures.
func ApplyPolicy(plan []Operation, checkpoint func([]Operation) error, apply func(Operation) error) (done []Operation, failed *Operation, err error) {
	for _, op := range plan {
		next := append(append([]Operation(nil), done...), op)
		if err = checkpoint(next); err != nil {
			return done, nil, err
		}
		if err = apply(op); err != nil {
			return done, &op, err
		}
		done = next
	}
	return done, nil, nil
}
