package main

import domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"

func seedDiplomas() []domainverification.Diploma {
	return []domainverification.Diploma{
		domainverification.MustNewDiploma(
			"dpl-001",
			"MSU",
			"2026-001",
			"Ivan Petrov",
			"Software Engineering",
			domainverification.StatusValid,
		),
		domainverification.MustNewDiploma(
			"dpl-002",
			"BMSTU",
			"2026-002",
			"Anna Sidorova",
			"Information Security",
			domainverification.StatusRevoked,
		),
	}
}
