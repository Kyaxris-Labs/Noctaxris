package store

import "sync"

var stripProductByStore sync.Map // *Store -> bool

// SetStripProduct toggles generic lab certificate Organization labeling for this store.
// When enabled, ACM/IoT lab certs use Organization "Lab" instead of "Noctaxris Lab".
func (s *Store) SetStripProduct(enabled bool) {
	if s == nil {
		return
	}
	stripProductByStore.Store(s, enabled)
}

func (s *Store) labCertificateOrganization() string {
	if s != nil {
		if v, ok := stripProductByStore.Load(s); ok {
			if enabled, _ := v.(bool); enabled {
				return "Lab"
			}
		}
	}
	return "Noctaxris Lab"
}
