package sources

import "context"

// ImportAndValidate is the user-facing import flow. Keeping orchestration in Go
// ensures validation continues if the settings page is unmounted while importing.
// Validation failure never rolls back a successfully encrypted import.
func (m *Manager) ImportAndValidate(ctx context.Context, path string) (ImportResult, error) {
	result, err := m.Import(ctx, path)
	if err != nil {
		return ImportResult{}, err
	}
	checked, err := m.Check(ctx, result.Source.ID)
	if err != nil {
		result.ValidationError = err.Error()
		return result, nil
	}
	result.Source = checked
	return result, nil
}
