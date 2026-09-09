import os
from pathlib import Path
import sys
import unittest
from unittest.mock import Mock, patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import cleanup_legacy_21_tag as cleanup


class LegacyTagCleanupTests(unittest.TestCase):
    def registry(self, alias):
        registry = Mock()
        registry.digest.side_effect = lambda tag: cleanup.DIGEST if tag == cleanup.VERSION else alias
        return registry

    def test_read_only_default_and_missing_alias(self):
        with patch.object(cleanup, "hub_request") as request:
            cleanup.migrate(self.registry(cleanup.DIGEST))
            cleanup.migrate(self.registry(None), delete=True)
            request.assert_not_called()

    def test_mismatch_prevents_login_and_deletion(self):
        for values in ((None, cleanup.DIGEST), (cleanup.DIGEST, "sha256:" + "b" * 64)):
            registry = Mock()
            registry.digest.side_effect = values
            with patch.object(cleanup, "hub_request") as request:
                with self.assertRaises(ValueError):
                    cleanup.migrate(registry, delete=True)
                request.assert_not_called()

    def test_only_exact_alias_is_deleted_with_before_after_checks(self):
        registry = Mock()
        registry.digest.side_effect = [cleanup.DIGEST, cleanup.DIGEST,
                                      cleanup.DIGEST, cleanup.DIGEST, cleanup.DIGEST, None]
        with patch.dict(os.environ, {"DOCKERHUB_USERNAME": "test", "DOCKERHUB_TOKEN": "test-secret"}), \
                patch.object(cleanup, "hub_request", side_effect=[{"access_token": "test-session"}, None]) as request:
            cleanup.migrate(registry, delete=True)
        self.assertEqual(request.call_count, 2)
        self.assertEqual(request.call_args.args,
                         (f"/v2/repositories/{cleanup.IMAGE}/tags/{cleanup.ALIAS}/", "DELETE"))
        self.assertEqual(registry.digest.call_count, 6)

    def test_retained_tag_or_manifest_delete_is_rejected(self):
        for path in (f"/v2/repositories/{cleanup.IMAGE}/tags/2.1/",
                     f"/v2/{cleanup.IMAGE}/manifests/{cleanup.DIGEST}"):
            with self.assertRaisesRegex(ValueError, "exact legacy"):
                cleanup.hub_request(path, "DELETE")


if __name__ == "__main__":
    unittest.main()
