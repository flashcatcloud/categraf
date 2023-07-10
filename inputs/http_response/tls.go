package http_response

import (
	"crypto/tls"
	"time"
)

func getEarliestCertExpiry(state *tls.ConnectionState) time.Time {
	earliest := time.Time{}
	for _, cert := range state.PeerCertificates {
		if (earliest.IsZero() || cert.NotAfter.Before(earliest)) && !cert.NotAfter.IsZero() {
			earliest = cert.NotAfter
		}
	}
	return earliest
}

func getCertName(state *tls.ConnectionState) string {
        for _, cert := range state.PeerCertificates {
                if !cert.IsCA {
                        return cert.Subject.CommonName
                }
        }
        return ""
}
