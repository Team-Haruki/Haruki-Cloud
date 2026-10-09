package handler

// normalizeCostume3DError is the reply for a failed costume command. The
// costume and 3D preview layers return typed errors for every case a user
// can act on; other failures get the generic reply.
func normalizeCostume3DError(err error) error {
	return WrapDomainError(err)
}
