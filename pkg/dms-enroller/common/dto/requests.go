package dto

type PostDmsCreationFormRequest struct {
	DmsName     string             `json:"name" validate:"required"`
	Subject     Subject            `json:"subject"`
	KeyMetadata PrivateKeyMetadata `json:"key_metadata" validate:"required"`
}

type PutChangeDmsStatusRequest struct {
	Status string   `json:"status" validate:"oneof='PENDING_APPROVAL' 'APPROVED'  'DENIED'  'REVOKED'"`
	CAs    []string `json:"authorized_cas"`
	ID     string   `validate:"required"`
}

type PostCSRRequest struct {
	Csr     string `json:"csr" validate:"required"`
	DmsName string `json:"name" validate:"required"`
}
