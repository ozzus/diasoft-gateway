package verification

import (
	"errors"
	"strings"
)

type Status string

const (
	StatusValid   Status = "valid"
	StatusRevoked Status = "revoked"
)

var (
	ErrEmptyID             = errors.New("empty diploma id")
	ErrEmptyUniversityCode = errors.New("empty university code")
	ErrEmptyDiplomaNumber  = errors.New("empty diploma number")
	ErrEmptyOwnerName      = errors.New("empty owner name")
	ErrEmptyProgram        = errors.New("empty program")
	ErrInvalidStatus       = errors.New("invalid diploma status")
)

type Diploma struct {
	id             string
	universityCode string
	number         string
	ownerName      string
	program        string
	status         Status
}

func NewDiploma(id, universityCode, number, ownerName, program string, status Status) (Diploma, error) {
	d := Diploma{
		id:             strings.TrimSpace(id),
		universityCode: strings.ToUpper(strings.TrimSpace(universityCode)),
		number:         strings.TrimSpace(number),
		ownerName:      strings.TrimSpace(ownerName),
		program:        strings.TrimSpace(program),
		status:         status,
	}

	switch {
	case d.id == "":
		return Diploma{}, ErrEmptyID
	case d.universityCode == "":
		return Diploma{}, ErrEmptyUniversityCode
	case d.number == "":
		return Diploma{}, ErrEmptyDiplomaNumber
	case d.ownerName == "":
		return Diploma{}, ErrEmptyOwnerName
	case d.program == "":
		return Diploma{}, ErrEmptyProgram
	case status != StatusValid && status != StatusRevoked:
		return Diploma{}, ErrInvalidStatus
	default:
		return d, nil
	}
}

func MustNewDiploma(id, universityCode, number, ownerName, program string, status Status) Diploma {
	d, err := NewDiploma(id, universityCode, number, ownerName, program, status)
	if err != nil {
		panic(err)
	}

	return d
}

func (d Diploma) ID() string             { return d.id }
func (d Diploma) UniversityCode() string { return d.universityCode }
func (d Diploma) Number() string         { return d.number }
func (d Diploma) OwnerName() string      { return d.ownerName }
func (d Diploma) Program() string        { return d.program }
func (d Diploma) Status() Status         { return d.status }
