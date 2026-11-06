package actualizar

import "crypto/ed25519"

// clavesOficiales son las claves publicas (Ed25519, base64) que firman las
// publicaciones. Vale la firma de cualquiera de las dos:
//
//   - la principal, cuya privada vive en el secreto FIRMA_ACTUALIZACIONES de
//     GitHub Actions;
//   - la de respaldo, cuya privada guarda el dueno fuera de linea. Si la
//     principal se pierde o se filtra, la siguiente version se firma con la
//     de respaldo y trae una principal nueva, sin ir caja por caja.
//
// Ver docs/ACTUALIZAR.md.
var clavesOficiales = []string{
	"MYPGwfgcW/GUY7lOUjNXsUBeJ2mQz0I+SPF9stfgZMI=",
	"6TIPdR8s3JybUvx2x196l+5DupdyDM680h5FhLx9cVE=",
}

// ClavesDelAgente son las claves que acepta este ejecutable: las oficiales y,
// solo en compilaciones de prueba (etiqueta actualizarprueba), una mas.
func ClavesDelAgente() []ed25519.PublicKey {
	return append(ClavesOficiales(), clavesDePrueba()...)
}
