import java.io.OutputStream;
import java.security.CodeSigner;
import java.security.MessageDigest;
import java.security.cert.X509Certificate;
import java.util.HashSet;
import java.util.HexFormat;
import java.util.Locale;
import java.util.jar.JarFile;
import javax.naming.ldap.LdapName;

// Java's JAR reader verifies the signature when each entry is read to EOF and
// associates that entry with its verified signer, including for self-signed keys.
public class VerifyBundle {
    public static void main(String[] args) throws Exception {
        var expected = HexFormat.of().parseHex(args[1]);
        var sha256 = MessageDigest.getInstance("SHA-256");
        var names = new HashSet<String>();
        boolean bundleConfig = false;
        boolean baseManifest = false;
        try (var bundle = new JarFile(args[0], true)) {
            var entries = bundle.entries();
            while (entries.hasMoreElements()) {
                var entry = entries.nextElement();
                var name = entry.getName();
                if (!names.add(name) || name.startsWith("/") || name.contains("\\") || name.contains("../")) {
                    throw new SecurityException("Invalid bundle entry name");
                }
                if (entry.isDirectory() || isSignatureMetadata(name)) {
                    continue;
                }
                try (var input = bundle.getInputStream(entry)) {
                    input.transferTo(OutputStream.nullOutputStream());
                }
                CodeSigner[] signers = entry.getCodeSigners();
                if (signers == null || signers.length != 1) {
                    throw new SecurityException("Bundle entry lacks one verified signer");
                }
                var certificate = (X509Certificate) signers[0].getSignerCertPath().getCertificates().get(0);
                if (!MessageDigest.isEqual(expected, sha256.digest(certificate.getEncoded()))) {
                    throw new SecurityException("Upload certificate fingerprint does not match");
                }
                certificate.checkValidity();
                for (var part : new LdapName(certificate.getSubjectX500Principal().getName()).getRdns()) {
                    if (part.getType().equalsIgnoreCase("CN") && part.getValue().toString().equalsIgnoreCase("Android Debug")) {
                        throw new SecurityException("Android debug certificate is not a release upload certificate");
                    }
                }
                bundleConfig |= name.equals("BundleConfig.pb");
                baseManifest |= name.equals("base/manifest/AndroidManifest.xml");
            }
        }
        if (!bundleConfig || !baseManifest) {
            throw new SecurityException("Missing signed Android App Bundle metadata");
        }
    }

    private static boolean isSignatureMetadata(String name) {
        var upper = name.toUpperCase(Locale.ROOT);
        if (!upper.startsWith("META-INF/")) {
            return false;
        }
        var file = upper.substring("META-INF/".length());
        return !file.contains("/") && (file.equals("MANIFEST.MF") || file.endsWith(".SF") ||
                file.endsWith(".RSA") || file.endsWith(".DSA") || file.endsWith(".EC") || file.startsWith("SIG-"));
    }
}
