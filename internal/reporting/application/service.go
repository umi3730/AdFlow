package application

import (
	"context"
	"github.com/umi3730/adflow/internal/reporting/domain"
	"time"
)

type Service struct{ reader domain.Reader }

func NewService(reader domain.Reader) *Service { return &Service{reader: reader} }
func (s *Service) Read(ctx context.Context, f domain.Filter) (domain.Report, error) {
	if err := f.Validate(); err != nil {
		return domain.Report{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := s.reader.ReadDeliveryRows(ctx, f)
	if err != nil {
		return domain.Report{}, err
	}
	return domain.Build(f, rows, time.Now()), nil
}
