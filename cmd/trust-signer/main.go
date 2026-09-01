// trust-signer is the offline tool behind the Haruki trust keyset.
//
// It never runs on the Cloud host. The operator generates the root Ed25519
// key on an offline machine, keeps the seed there, and uses this tool to sign
// keyset documents that the Cloud then serves verbatim and clients verify with
// the pinned root public key.
//
//	trust-signer keygen --out root.seed --key-id root-2026
//	trust-signer sign   --key root.seed --key-id root-2026 --domain keyset \
//	                    --in keyset.json --out keyset.signed.json
//	trust-signer verify --public <hex> --in keyset.signed.json --domain keyset
//
// The same tool also generates the online manifest signing key that the Cloud
// loads through haruki_bot.manifest_signing_key; its public half goes into the
// keyset under manifest_signing_keys.
package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"haruki-cloud/internal/core/trustsign"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "keygen":
		err = runKeygen(args[1:], stdout)
	case "sign":
		err = runSign(args[1:], stdout)
	case "verify":
		err = runVerify(args[1:], stdout)
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s\n", args[0], usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

const usage = `usage: trust-signer <keygen|sign|verify> [flags]

  keygen --out <seed-file> --key-id <id>
      Generate an Ed25519 seed, write it (hex, mode 0600) to <seed-file> and
      print the key id and public key. Refuses to overwrite an existing file.

  sign --key <seed-file> --key-id <id> --domain <keyset|manifest>
       --in <payload-file> --out <envelope-file> [--encoding json]
      Sign the exact bytes of <payload-file> and write a trustsign envelope.
      For the keyset domain the payload is validated as a KeysetDocument.

  verify --public <hex> --in <envelope-file> [--domain <keyset|manifest>]
      Verify an envelope and print its payload to stdout.`

func runKeygen(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	out := fs.String("out", "", "file to write the hex seed to (required)")
	keyID := fs.String("key-id", "", "stable identifier for this key (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" || strings.TrimSpace(*keyID) == "" {
		return errors.New("keygen: --out and --key-id are required")
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("keygen: %s already exists; refusing to overwrite a key", *out)
	}
	seed, err := trustsign.GenerateSeed()
	if err != nil {
		return err
	}
	signer, err := trustsign.NewSigner(*keyID, seed)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, []byte(hex.EncodeToString(seed)+"\n"), 0o600); err != nil {
		return fmt.Errorf("keygen: write seed: %w", err)
	}
	return json.NewEncoder(stdout).Encode(map[string]string{
		"key_id":     signer.KeyID(),
		"public_key": signer.PublicKeyHex(),
		"seed_file":  *out,
	})
}

func runSign(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	keyFile := fs.String("key", "", "seed file written by keygen (required)")
	keyID := fs.String("key-id", "", "key id recorded in the envelope (required)")
	domain := fs.String("domain", "", "keyset or manifest (required)")
	in := fs.String("in", "", "payload file to sign (required)")
	out := fs.String("out", "", "envelope file to write (required)")
	encoding := fs.String("encoding", trustsign.EncodingJSON, "payload encoding hint: json or msgpack")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *keyFile == "" || strings.TrimSpace(*keyID) == "" || *domain == "" || *in == "" || *out == "" {
		return errors.New("sign: --key, --key-id, --domain, --in and --out are required")
	}
	domainName, err := resolveDomain(*domain)
	if err != nil {
		return err
	}
	if *encoding != trustsign.EncodingJSON && *encoding != trustsign.EncodingMsgPack {
		return fmt.Errorf("sign: unsupported encoding %q", *encoding)
	}
	seedHex, err := os.ReadFile(*keyFile)
	if err != nil {
		return fmt.Errorf("sign: read seed: %w", err)
	}
	signer, err := trustsign.NewSignerFromHex(*keyID, string(seedHex))
	if err != nil {
		return err
	}
	payload, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("sign: read payload: %w", err)
	}
	if domainName == trustsign.DomainKeyset {
		if *encoding != trustsign.EncodingJSON {
			return errors.New("sign: keyset payloads must be JSON")
		}
		var doc trustsign.KeysetDocument
		if err := json.Unmarshal(payload, &doc); err != nil {
			return fmt.Errorf("sign: keyset payload is not valid JSON: %w", err)
		}
		if err := doc.Validate(); err != nil {
			return fmt.Errorf("sign: keyset payload rejected: %w", err)
		}
	}
	envelope, err := signer.Sign(domainName, *encoding, payload)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("sign: write envelope: %w", err)
	}
	return json.NewEncoder(stdout).Encode(map[string]any{
		"key_id":        signer.KeyID(),
		"public_key":    signer.PublicKeyHex(),
		"domain":        domainName,
		"payload_bytes": len(payload),
		"envelope_file": *out,
	})
}

func runVerify(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	pubHex := fs.String("public", "", "hex-encoded Ed25519 public key (required)")
	in := fs.String("in", "", "envelope file (required)")
	domain := fs.String("domain", "", "expected domain: keyset or manifest (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pubHex == "" || *in == "" {
		return errors.New("verify: --public and --in are required")
	}
	pub, err := trustsign.ParsePublicKeyHex(*pubHex)
	if err != nil {
		return err
	}
	wantDomain := ""
	if *domain != "" {
		if wantDomain, err = resolveDomain(*domain); err != nil {
			return err
		}
	}
	raw, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("verify: read envelope: %w", err)
	}
	var envelope trustsign.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("verify: envelope is not valid JSON: %w", err)
	}
	if err := trustsign.Verify(pub, envelope, wantDomain); err != nil {
		return err
	}
	_, err = stdout.Write(envelope.Payload)
	return err
}

func resolveDomain(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "keyset", trustsign.DomainKeyset:
		return trustsign.DomainKeyset, nil
	case "manifest", trustsign.DomainManifest:
		return trustsign.DomainManifest, nil
	}
	return "", fmt.Errorf("unknown domain %q (want keyset or manifest)", name)
}
