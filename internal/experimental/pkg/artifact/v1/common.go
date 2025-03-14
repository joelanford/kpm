package v1

const AnnotationPackageName = "olm.operatorframework.io/package-name"

type NamedURL struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type NamedEmail struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}
