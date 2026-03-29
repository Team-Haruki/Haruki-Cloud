package onebot11

type ReplayError TextData

func (r ReplayError) Error() string {
	return r.Text
}
