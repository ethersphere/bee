package dynamicaccess

type Controller interface {
}

type defaultController struct {
	histrory History
	uploader Publish
	grantee  Grantee
}

func NewController(histrory History, uploader Publish, grantee Grantee) Controller {
	return &defaultController{
		histrory: histrory,
		uploader: uploader,
		grantee:  grantee,
	}
}
