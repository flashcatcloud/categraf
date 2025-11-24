package x509_cert

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pion/dtls/v2"

	"golang.org/x/crypto/ocsp"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/globpath"
	"flashcat.cloud/categraf/pkg/proxy"
	commontls "flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/types"
)

const inputName = "x509_cert"

// Regexp for handling file URIs containing a drive letter and leading slash
var reDriveLetter = regexp.MustCompile(`^/([a-zA-Z]:/)`)

type X509Cert struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

type Instance struct {
	Targets          []string        `toml:"targets"`
	Timeout          config.Duration `toml:"timeout"`
	ServerName       string          `toml:"server_name"`
	ExcludeRootCerts bool            `toml:"exclude_root_certs"`

	globPaths []globpath.GlobPath
	locations []*url.URL

	classification map[string]string

	commontls.ClientConfig
	tlsCfg *tls.Config

	proxy.TCPProxy
	client *http.Client
	config.HTTPCommonConfig
	config.InstanceConfig
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &X509Cert{}
	})
}

func (ins *Instance) Init() error {
	if len(ins.Targets) == 0 {
		return types.ErrInstancesEmpty
	}

	if ins.Timeout <= 0 {
		ins.Timeout = config.Duration(time.Second * 5)
	}

	if ins.ClientConfig.ServerName != "" && ins.ServerName != "" {
		return fmt.Errorf("both server_name (%q) and tls_server_name (%q) are set, but they are mutually exclusive", ins.ServerName, ins.ClientConfig.ServerName)
	} else if ins.ServerName != "" {
		// Store the user-provided server-name in the TLS configuration
		ins.ClientConfig.ServerName = ins.ServerName
	}

	// Normalize the sources, handle files and file-globbing
	if err := ins.sourcesToURLs(); err != nil {
		return err
	}

	ins.InitHTTPClientConfig()

	var err error
	ins.client, err = ins.createHTTPClient()

	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return err
	}
	if tlsCfg == nil {
		tlsCfg = &tls.Config{}
	}
	ins.tlsCfg = tlsCfg
	return err
}

func (ins *X509Cert) Clone() inputs.Input {
	return &X509Cert{}
}

func (ins *X509Cert) Name() string {
	return inputName
}

func (ins *X509Cert) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(ins.Instances))
	for i := 0; i < len(ins.Instances); i++ {
		ret[i] = ins.Instances[i]
	}
	return ret
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if len(ins.Targets) == 0 {
		return
	}

	if err := ins.sourcesToURLs(); err != nil {
		log.Printf("E! failed to update sources: %v", err)
		return
	}

	now := time.Now()
	collectedUrls := append(ins.locations, ins.collectCertURLs()...)
	for _, location := range collectedUrls {
		certs, ocspresp, err := ins.getCert(location, time.Duration(ins.Timeout))
		if err != nil {
			log.Printf("E! cannot get SSL cert %q: %v", location, err)
			continue
		}

		intermediates := x509.NewCertPool()
		if len(certs) > 1 {
			for _, cert := range certs[1:] {
				intermediates.AddCert(cert)
			}
		}

		dnsName := ins.serverName(location)
		results := make([]error, len(certs))
		ins.classification = make(map[string]string)
		for i, cert := range certs {
			opts := x509.VerifyOptions{
				Intermediates: intermediates,
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
				Roots:         ins.tlsCfg.RootCAs,
				DNSName:       dnsName,
			}
			results[i] = ins.processCertificate(cert, opts)
			dnsName = ""
		}

		for i, cert := range certs {
			fields := getFields(cert, now)
			tags := getTags(cert, location.String())

			if err := results[i]; err == nil {
				tags["verification"] = "valid"
				fields["verification_code"] = 0
			} else {
				tags["verification"] = "invalid"
				fields["verification_code"] = 1
			}

			if i == 0 && ocspresp != nil && len(*ocspresp) > 0 {
				var ocspissuer *x509.Certificate
				for _, chaincert := range certs[1:] {
					if cert.Issuer.CommonName == chaincert.Subject.CommonName &&
						cert.Issuer.SerialNumber == chaincert.Subject.SerialNumber {
						ocspissuer = chaincert
						break
					}
				}
				resp, err := ocsp.ParseResponse(*ocspresp, ocspissuer)
				if err != nil {
					if ocspissuer == nil {
						tags["ocsp_stapled"] = "no"
					} else {
						ocspissuer = nil // retry parsing w/out issuer cert
						resp, err = ocsp.ParseResponse(*ocspresp, ocspissuer)
					}
				}
				if err != nil {
					tags["ocsp_stapled"] = "no"
				} else {
					tags["ocsp_stapled"] = "yes"
					if ocspissuer != nil {
						tags["ocsp_verified"] = "yes"
					} else {
						tags["ocsp_verified"] = "no"
					}
					// resp.Status: 0=Good 1=Revoked 2=Unknown
					fields["ocsp_status_code"] = resp.Status
					switch resp.Status {
					case 0:
						tags["ocsp_status"] = "good"
					case 1:
						tags["ocsp_status"] = "revoked"
						// Status=Good: revoked_at always = -62135596800
						fields["ocsp_revoked_at"] = resp.RevokedAt.Unix()
					default:
						tags["ocsp_status"] = "unknown"
					}
					fields["ocsp_produced_at"] = resp.ProducedAt.Unix()
					fields["ocsp_this_update"] = resp.ThisUpdate.Unix()
					fields["ocsp_next_update"] = resp.NextUpdate.Unix()
				}
			} else {
				tags["ocsp_stapled"] = "no"
			}

			sig := hex.EncodeToString(cert.Signature)
			if class, found := ins.classification[sig]; found {
				tags["type"] = class
			} else {
				tags["type"] = "leaf"
			}

			slist.PushSamples("x509_cert", fields, tags)
			if ins.ExcludeRootCerts {
				break
			}
		}
	}
}

func (ins *Instance) processCertificate(cert *x509.Certificate, opts x509.VerifyOptions) error {
	chains, err := cert.Verify(opts)
	if err != nil {
		if ins.DebugMod {
			log.Printf("W! Invalid certificate %v: %v", cert.SerialNumber.Text(16), err)
			log.Printf("W! cert DNS names:    %v", cert.DNSNames)
			log.Printf("W! cert IP addresses: %v", cert.IPAddresses)
			log.Printf("W! cert subject:      %v", cert.Subject)
			log.Printf("W! cert issuer:       %v", cert.Issuer)
			log.Printf("W! opts.DNSName:      %v", opts.DNSName)
			log.Printf("W! verify options:    %v", opts)
			log.Printf("W! verify error:      %v", err)
			log.Printf("W! tlsCfg.ServerName: %v", ins.tlsCfg.ServerName)
			log.Printf("W! ServerName:        %v", ins.ServerName)
		}
	}

	// Check if the certificate is a root-certificate.
	// The only reliable way to distinguish root certificates from
	// intermediates is the fact that root certificates are self-signed,
	// i.e. you can verify the certificate with its own public key.
	rootErr := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature)
	if rootErr == nil {
		sig := hex.EncodeToString(cert.Signature)
		ins.classification[sig] = "root"
	}
	// Identify intermediate certificates
	for _, chain := range chains {
		for _, chainCert := range chain[1:] {
			sig := hex.EncodeToString(chainCert.Signature)
			if _, found := ins.classification[sig]; !found {
				if chainCert.IsCA {
					ins.classification[sig] = "intermediate"
				} else {
					ins.classification[sig] = "unknown"
				}
			}
		}
	}

	return err
}

func (ins *Instance) sourcesToURLs() error {
	ins.locations = []*url.URL{}
	for _, target := range ins.Targets {
		if strings.HasPrefix(target, "file://") || strings.HasPrefix(target, "/") {
			target = filepath.ToSlash(strings.TrimPrefix(target, "file://"))
			target = reDriveLetter.ReplaceAllString(target, "$1")
			files, err := filepath.Glob(target)
			if err != nil {
				return fmt.Errorf("could not process target %q: %w", target, err)
			}
			for _, file := range files {
				ins.locations = append(ins.locations, &url.URL{Scheme: "file", Path: file})
			}
		} else {
			if strings.Index(target, ":\\") == 1 {
				target = "file://" + filepath.ToSlash(target)
			}
			u, err := url.Parse(target)
			if err != nil {
				return fmt.Errorf("failed to parse target %q: %w", target, err)
			}
			ins.locations = append(ins.locations, u)
		}
	}
	return nil
}

func (ins *Instance) serverName(u *url.URL) string {
	if ins.ServerName != "" {
		return ins.ServerName
	}
	return u.Hostname()
}

func (ins *Instance) getCert(u *url.URL, timeout time.Duration) ([]*x509.Certificate, *[]byte, error) {
	protocol := u.Scheme
	switch u.Scheme {
	case "udp", "udp4", "udp6":
		ipConn, err := net.DialTimeout(u.Scheme, u.Host, timeout)
		if err != nil {
			return nil, nil, err
		}
		defer ipConn.Close()

		dtlsCfg := &dtls.Config{
			InsecureSkipVerify: true,
			Certificates:       ins.tlsCfg.Certificates,
			RootCAs:            ins.tlsCfg.RootCAs,
			ServerName:         ins.serverName(u),
		}
		conn, err := dtls.Client(ipConn, dtlsCfg)
		if err != nil {
			return nil, nil, err
		}
		defer conn.Close()

		rawCerts := conn.ConnectionState().PeerCertificates
		var certs []*x509.Certificate
		for _, rawCert := range rawCerts {
			parsed, err := x509.ParseCertificate(rawCert)
			if err != nil {
				return nil, nil, err
			}

			if parsed != nil {
				certs = append(certs, parsed)
			}
		}

		return certs, nil, nil
	case "https":
		protocol = "tcp"
		if u.Port() == "" {
			u.Host += ":443"
		}
		fallthrough
	case "tcp", "tcp4", "tcp6":
		dialer, err := ins.Proxy()
		if err != nil {
			return nil, nil, err
		}
		ipConn, err := dialer.DialTimeout(protocol, u.Host, timeout)
		if err != nil {
			return nil, nil, err
		}
		defer ipConn.Close()

		downloadTLSCfg := ins.tlsCfg.Clone()
		downloadTLSCfg.ServerName = ins.serverName(u)
		downloadTLSCfg.InsecureSkipVerify = true

		conn := tls.Client(ipConn, downloadTLSCfg)
		defer conn.Close()

		hsErr := conn.Handshake()
		if hsErr != nil {
			return nil, nil, hsErr
		}

		certs := conn.ConnectionState().PeerCertificates
		ocspresp := conn.ConnectionState().OCSPResponse

		return certs, &ocspresp, nil
	case "file":
		content, err := os.ReadFile(u.Path)
		if err != nil {
			return nil, nil, err
		}
		var certs []*x509.Certificate
		for {
			block, rest := pem.Decode(bytes.TrimSpace(content))
			if block == nil {
				return nil, nil, errors.New("failed to parse certificate PEM")
			}

			if block.Type == "CERTIFICATE" {
				cert, err := x509.ParseCertificate(block.Bytes)
				if err != nil {
					return nil, nil, err
				}
				certs = append(certs, cert)
			}
			if len(rest) == 0 {
				break
			}
			content = rest
		}
		return certs, nil, nil
	case "smtp":
		ipConn, err := net.DialTimeout("tcp", u.Host, timeout)
		if err != nil {
			return nil, nil, err
		}
		defer ipConn.Close()

		downloadTLSCfg := ins.tlsCfg.Clone()
		downloadTLSCfg.ServerName = ins.serverName(u)
		downloadTLSCfg.InsecureSkipVerify = true

		smtpConn, err := smtp.NewClient(ipConn, u.Host)
		if err != nil {
			return nil, nil, err
		}

		err = smtpConn.Hello(downloadTLSCfg.ServerName)
		if err != nil {
			return nil, nil, err
		}

		id, err := smtpConn.Text.Cmd("STARTTLS")
		if err != nil {
			return nil, nil, err
		}

		smtpConn.Text.StartResponse(id)
		defer smtpConn.Text.EndResponse(id)
		_, _, err = smtpConn.Text.ReadResponse(220)
		if err != nil {
			return nil, nil, fmt.Errorf("did not get 220 after STARTTLS: %w", err)
		}

		tlsConn := tls.Client(ipConn, downloadTLSCfg)
		defer tlsConn.Close()

		hsErr := tlsConn.Handshake()
		if hsErr != nil {
			return nil, nil, hsErr
		}

		certs := tlsConn.ConnectionState().PeerCertificates
		ocspresp := tlsConn.ConnectionState().OCSPResponse

		return certs, &ocspresp, nil
	default:
		return nil, nil, fmt.Errorf("unsupported scheme %q in location %s", u.Scheme, u.String())
	}
}

func getFields(cert *x509.Certificate, now time.Time) map[string]interface{} {
	age := int(now.Sub(cert.NotBefore).Seconds())
	expiry := int(cert.NotAfter.Sub(now).Seconds())
	startdate := cert.NotBefore.Unix()
	enddate := cert.NotAfter.Unix()

	fields := map[string]interface{}{
		"age":       age,
		"expiry":    expiry,
		"startdate": startdate,
		"enddate":   enddate,
	}

	return fields
}

func getTags(cert *x509.Certificate, location string) map[string]string {
	tags := map[string]string{
		"target":               location,
		"common_name":          cert.Subject.CommonName,
		"serial_number":        cert.SerialNumber.Text(16),
		"signature_algorithm":  cert.SignatureAlgorithm.String(),
		"public_key_algorithm": cert.PublicKeyAlgorithm.String(),
	}

	if len(cert.Subject.Organization) > 0 {
		tags["organization"] = cert.Subject.Organization[0]
	}
	if len(cert.Subject.OrganizationalUnit) > 0 {
		tags["organizational_unit"] = cert.Subject.OrganizationalUnit[0]
	}
	if len(cert.Subject.Country) > 0 {
		tags["country"] = cert.Subject.Country[0]
	}
	if len(cert.Subject.Province) > 0 {
		tags["province"] = cert.Subject.Province[0]
	}
	if len(cert.Subject.Locality) > 0 {
		tags["locality"] = cert.Subject.Locality[0]
	}

	tags["issuer_common_name"] = cert.Issuer.CommonName
	tags["issuer_serial_number"] = cert.Issuer.SerialNumber

	san := append(cert.DNSNames, cert.EmailAddresses...)
	for _, ip := range cert.IPAddresses {
		san = append(san, ip.String())
	}
	for _, uri := range cert.URIs {
		san = append(san, uri.String())
	}
	tags["san"] = strings.Join(san, ",")

	return tags
}

func (ins *Instance) createHTTPClient() (*http.Client, error) {
	tr := &http.Transport{
		ResponseHeaderTimeout: time.Duration(ins.Timeout),
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(ins.Timeout),
	}
	return client, nil
}

func (ins *Instance) collectCertURLs() []*url.URL {
	var urls []*url.URL

	for _, path := range ins.globPaths {
		files := path.Match()
		if len(files) == 0 {
			log.Println("W! could not find file:", path.GetRoots())
			continue
		}
		for _, file := range files {
			fn := filepath.ToSlash(file)
			urls = append(urls, &url.URL{Scheme: "file", Path: fn})
		}
	}

	return urls
}
