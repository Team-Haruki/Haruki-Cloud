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
// keyset under manifest_signing_keys, and the delegated release signing key
// whose public half goes under release_signing_keys:
//
//	trust-signer keygen --out release.seed --key-id release-2026-10
//	trust-signer keyset-add-release-key --in keyset-v3.json --out keyset-v4.json \
//	                    --seed release.seed --key-id release-2026-10 --not-after 2027-04-01T00:00:00Z
//	trust-signer sign   --key root.seed --key-id root-2026 --domain keyset \
//	                    --in keyset-v4.json --out keyset-v4.signed.json
//	trust-signer sign   --key release.seed --key-id release-2026-10 --domain release \
//	                    --in build-policy.json --out build-policy.signed.json
//	trust-signer verify --public <root hex> --keyset keyset-v4.signed.json \
//	                    --in build-policy.signed.json --domain release
package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/core/buildpolicy"
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
	case "keyset-add-release-key":
		err = runKeysetAddReleaseKey(args[1:], stdout)
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

const usage = `usage: trust-signer <keygen|sign|verify|keyset-add-release-key> [flags]

  keygen --out <seed-file> --key-id <id>
      Generate an Ed25519 seed, write it (hex, mode 0600) to <seed-file> and
      print the key id and public key. Refuses to overwrite an existing file.

  sign --key <seed-file> --key-id <id> --domain <keyset|manifest|release>
       --in <payload-file> --out <envelope-file> [--encoding json]
      Sign the exact bytes of <payload-file> and write a trustsign envelope.
      For the keyset domain the payload is validated as a KeysetDocument;
      for the release domain as a buildpolicy.Document (client build policy).

  verify --public <hex> --in <envelope-file> [--domain <keyset|manifest|release>]
         [--keyset <signed-keyset-file>]
      Verify an envelope and print its payload to stdout. With --keyset (release
      domain only) the envelope is checked the way the Cloud does: against the
      release_signing_keys of the keyset after that keyset verified under --public,
      or against --public itself when the key id is not delegated.

  keyset-add-release-key --in <keyset-file> --out <keyset-file> --seed <seed-file>
         --key-id <id> --not-after <unix|RFC3339> [--not-before <unix|RFC3339>]
         [--version <n>]
      Copy a keyset document, append the public key of <seed-file> to
      release_signing_keys, bump version (default: previous + 1) and refresh
      issued_at. Root-sign the result with "sign --domain keyset".`

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
	domain := fs.String("domain", "", "keyset, manifest or release (required)")
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
	if domainName == trustsign.DomainRelease {
		if *encoding != trustsign.EncodingJSON {
			return errors.New("sign: release policy payloads must be JSON")
		}
		if _, err := buildpolicy.Parse(payload, nil); err != nil {
			return fmt.Errorf("sign: release policy payload rejected: %w", err)
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
	domain := fs.String("domain", "", "expected domain: keyset, manifest or release (optional)")
	keysetFile := fs.String("keyset", "", "signed keyset whose release_signing_keys may have signed a release envelope (optional)")
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
	if *keysetFile != "" {
		if wantDomain != trustsign.DomainRelease {
			return errors.New("verify: --keyset applies to --domain release only")
		}
		return verifyDelegatedRelease(pub, *keysetFile, raw, stdout)
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

// verifyDelegatedRelease mirrors the Cloud's policy loading: the keyset must
// verify under the root key, then the envelope under the release key its
// key_id names (or the root when the id is not delegated).
func verifyDelegatedRelease(root []byte, keysetFile string, raw []byte, stdout io.Writer) error {
	keysetRaw, err := os.ReadFile(keysetFile)
	if err != nil {
		return fmt.Errorf("verify: read keyset: %w", err)
	}
	keyset, err := trustsign.ParseKeyset(keysetRaw, root)
	if err != nil {
		return fmt.Errorf("verify: keyset rejected: %w", err)
	}
	var envelope trustsign.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("verify: envelope is not valid JSON: %w", err)
	}
	if _, err := buildpolicy.Parse(raw, &buildpolicy.Verifier{Root: root, Keyset: keyset}); err != nil {
		return err
	}
	_, err = stdout.Write(envelope.Payload)
	return err
}

func runKeysetAddReleaseKey(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("keyset-add-release-key", flag.ContinueOnError)
	in := fs.String("in", "", "keyset document (plain JSON, not the signed envelope) to extend (required)")
	out := fs.String("out", "", "keyset document to write (required, must not exist)")
	seedFile := fs.String("seed", "", "release key seed file written by keygen (required)")
	keyID := fs.String("key-id", "", "key id the release key signs under (required)")
	notBefore := fs.String("not-before", "", "start of the key's validity window, unix seconds or RFC3339 (optional)")
	notAfter := fs.String("not-after", "", "end of the key's validity window, unix seconds or RFC3339 (required)")
	version := fs.Int64("version", 0, "keyset version to write (default: previous + 1)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" || *out == "" || *seedFile == "" || strings.TrimSpace(*keyID) == "" || *notAfter == "" {
		return errors.New("keyset-add-release-key: --in, --out, --seed, --key-id and --not-after are required")
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("keyset-add-release-key: %s already exists; refusing to overwrite", *out)
	}
	after, err := parseUnixTime(*notAfter)
	if err != nil {
		return fmt.Errorf("keyset-add-release-key: --not-after: %w", err)
	}
	var before int64
	if *notBefore != "" {
		if before, err = parseUnixTime(*notBefore); err != nil {
			return fmt.Errorf("keyset-add-release-key: --not-before: %w", err)
		}
	}
	seedHex, err := os.ReadFile(*seedFile)
	if err != nil {
		return fmt.Errorf("keyset-add-release-key: read seed: %w", err)
	}
	signer, err := trustsign.NewSignerFromHex(*keyID, string(seedHex))
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("keyset-add-release-key: read keyset: %w", err)
	}
	var doc trustsign.KeysetDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("keyset-add-release-key: keyset is not valid JSON: %w", err)
	}
	if *version == 0 {
		*version = doc.Version + 1
	}
	if *version <= doc.Version {
		return fmt.Errorf("keyset-add-release-key: version %d must exceed the current %d", *version, doc.Version)
	}
	doc.Version = *version
	doc.IssuedAt = time.Now().Unix()
	doc.ReleaseSigningKeys = append(doc.ReleaseSigningKeys, trustsign.KeysetKey{
		KeyID:     signer.KeyID(),
		PublicKey: signer.PublicKeyHex(),
		NotBefore: before,
		NotAfter:  after,
	})
	if err := doc.Validate(); err != nil {
		return fmt.Errorf("keyset-add-release-key: resulting keyset rejected: %w", err)
	}
	if after > doc.ExpiresAt {
		fmt.Fprintf(stdout, "note: not_after is later than the keyset expires_at; the key is usable only until the keyset is re-signed\n")
	}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("keyset-add-release-key: write keyset: %w", err)
	}
	return json.NewEncoder(stdout).Encode(map[string]any{
		"key_id":       signer.KeyID(),
		"public_key":   signer.PublicKeyHex(),
		"not_before":   before,
		"not_after":    after,
		"version":      doc.Version,
		"keyset_file":  *out,
		"release_keys": len(doc.ReleaseSigningKeys),
	})
}

// parseUnixTime accepts unix seconds or an RFC3339 timestamp.
func parseUnixTime(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if unix <= 0 {
			return 0, errors.New("must be positive")
		}
		return unix, nil
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0, fmt.Errorf("want unix seconds or RFC3339: %w", err)
	}
	return ts.Unix(), nil
}

func resolveDomain(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "keyset", trustsign.DomainKeyset:
		return trustsign.DomainKeyset, nil
	case "manifest", trustsign.DomainManifest:
		return trustsign.DomainManifest, nil
	case "release", trustsign.DomainRelease:
		return trustsign.DomainRelease, nil
	}
	return "", fmt.Errorf("unknown domain %q (want keyset, manifest or release)", name)
}
