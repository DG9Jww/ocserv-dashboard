package federation

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/internal/models"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/repository"
	"gorm.io/gorm"
)

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(err)
	}
	return n.Add(n, big.NewInt(1))
}

func (m *Master) trust(ctx context.Context, scope string) (Trust, error) {
	var result Trust
	err := m.store.Transaction(ctx, func(tx *repository.PortalRepository) error {
		ca, err := m.ensureCA(ctx, tx, scope)
		if err != nil {
			return err
		}
		result = Trust{CA: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Cert}), CRL: pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: ca.CRL})}
		return nil
	})
	return result, err
}

func (m *Master) ensureCA(ctx context.Context, tx *repository.PortalRepository, scope string) (models.PortalSigningCA, error) {
	ca, err := tx.CA(ctx, scope)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ca, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return ca, err
	}
	keyData, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return ca, err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "Ocserv client CA " + scope},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(10, 0, 0), BasicConstraintsValid: true, IsCA: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	cert, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return ca, err
	}
	issuer, err := x509.ParseCertificate(cert)
	if err != nil {
		return ca, err
	}
	crl, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(1), ThisUpdate: issuer.NotBefore, NextUpdate: issuer.NotAfter,
	}, issuer, key)
	if err != nil {
		return ca, err
	}
	ca = models.PortalSigningCA{Scope: scope, Cert: cert, CRL: crl, Key: m.encrypt(keyData, "ca:"+scope)}
	return ca, tx.SaveNew(ctx, &ca)
}

func (m *Master) PKCS12(ctx context.Context, customer models.PortalCustomer, scope string) ([]byte, error) {
	row, err := m.store.Certificate(ctx, customer.ID, scope)
	if err == nil {
		cert, parseErr := x509.ParseCertificate(row.Cert)
		if parseErr != nil {
			return nil, parseErr
		}
		if time.Now().After(cert.NotAfter) {
			err = gorm.ErrRecordNotFound
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row, err = m.issue(ctx, customer, scope)
	}
	if err != nil {
		return nil, err
	}
	key, err := m.decrypt(row.Key, "cert:"+customer.ID+":"+scope)
	if err != nil {
		return nil, err
	}
	password, err := m.decrypt(customer.PasswordBox, "password:"+customer.Username)
	if err != nil {
		return nil, err
	}
	ca, err := m.store.CA(ctx, scope)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "ocserv-p12-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	certFile, keyFile, caFile := filepath.Join(dir, "client.pem"), filepath.Join(dir, "key.pem"), filepath.Join(dir, "ca.pem")
	for _, item := range []struct {
		path, kind string
		data       []byte
	}{{certFile, "CERTIFICATE", row.Cert}, {keyFile, "RSA PRIVATE KEY", key}, {caFile, "CERTIFICATE", ca.Cert}} {
		if err := os.WriteFile(item.path, pem.EncodeToMemory(&pem.Block{Type: item.kind, Bytes: item.data}), 0600); err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, "openssl", "pkcs12", "-export", "-legacy", "-keypbe", "PBE-SHA1-3DES", "-certpbe", "PBE-SHA1-3DES", "-macalg", "sha1", "-inkey", keyFile, "-in", certFile, "-certfile", caFile, "-name", "Ocserv "+customer.Username, "-passout", "stdin")
	cmd.Stdin = bytes.NewReader(append(password, '\n'))
	return cmd.Output()
}

func (m *Master) issue(ctx context.Context, customer models.PortalCustomer, scope string) (models.PortalClientCertificate, error) {
	var row models.PortalClientCertificate
	err := m.store.Transaction(ctx, func(tx *repository.PortalRepository) error {
		current, err := tx.Customer(ctx, customer.ID)
		if err != nil {
			return err
		}
		if current.DeletedAt != nil || current.Locked {
			return ErrDefault
		}
		now := time.Now().UTC()
		existing, err := tx.Certificate(ctx, customer.ID, scope)
		if err == nil {
			parsed, err := x509.ParseCertificate(existing.Cert)
			if err != nil {
				return err
			}
			if now.Before(parsed.NotAfter) {
				row = existing
				return nil
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		ca, err := m.ensureCA(ctx, tx, scope)
		if err != nil {
			return err
		}
		issuer, err := x509.ParseCertificate(ca.Cert)
		if err != nil {
			return err
		}
		keyData, err := m.decrypt(ca.Key, "ca:"+scope)
		if err != nil {
			return err
		}
		key, err := x509.ParseECPrivateKey(keyData)
		if err != nil {
			return err
		}
		clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return err
		}
		expiry := now.AddDate(0, 0, 825)
		if expiry.After(issuer.NotAfter) {
			expiry = issuer.NotAfter
		}
		tmpl := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: customer.Username}, NotBefore: now.Add(-time.Hour),
			NotAfter: expiry, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		cert, err := x509.CreateCertificate(rand.Reader, tmpl, issuer, &clientKey.PublicKey, key)
		if err != nil {
			return err
		}
		row = models.PortalClientCertificate{CustomerID: customer.ID, Scope: scope, Cert: cert, Key: m.encrypt(x509.MarshalPKCS1PrivateKey(clientKey), "cert:"+customer.ID+":"+scope)}
		return tx.Save(ctx, &row)
	})
	return row, err
}

func (m *Master) revokeCustomerCertificates(ctx context.Context, tx *repository.PortalRepository, id string) error {
	certificates, err := tx.CustomerCertificates(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, saved := range certificates {
		cert, err := x509.ParseCertificate(saved.Cert)
		if err != nil {
			return err
		}
		if !now.Before(cert.NotAfter) {
			continue
		}
		ca, err := tx.CA(ctx, saved.Scope)
		if err != nil {
			return err
		}
		issuer, err := x509.ParseCertificate(ca.Cert)
		if err != nil {
			return err
		}
		list, err := x509.ParseRevocationList(ca.CRL)
		if err != nil {
			return err
		}
		keyBytes, err := m.decrypt(ca.Key, "ca:"+saved.Scope)
		if err != nil {
			return err
		}
		key, err := x509.ParseECPrivateKey(keyBytes)
		if err != nil {
			return err
		}
		ca.CRL, err = x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
			Number:     new(big.Int).Add(list.Number, big.NewInt(1)),
			ThisUpdate: now, NextUpdate: issuer.NotAfter,
			RevokedCertificateEntries: append(list.RevokedCertificateEntries, x509.RevocationListEntry{SerialNumber: cert.SerialNumber, RevocationTime: now}),
		}, issuer, key)
		if err != nil {
			return err
		}
		if err := tx.Save(ctx, &ca); err != nil {
			return err
		}
	}
	return nil
}
