package shared

// RowScanner abstracts pgx.Row and pgx.Rows so scan helpers can accept both.
type RowScanner interface {
	Scan(dest ...any) error
}
