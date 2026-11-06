import importlib.util
from pathlib import Path
import unittest
from urllib.error import HTTPError

spec = importlib.util.spec_from_file_location("publisher", Path(__file__).resolve().parents[1] / "scripts/publish_release.py")
publisher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publisher)


class TagBindingTests(unittest.TestCase):
    def test_matching_lightweight_tag(self):
        publisher.require_matching_tag(lambda _: {"object": {"type": "commit", "sha": "expected"}}, "owner/repo", "v1", "expected", allow_missing=False)

    def test_wrong_existing_tag_never_accepted_as_target_commitish(self):
        with self.assertRaisesRegex(ValueError, "different source"):
            publisher.require_matching_tag(lambda _: {"object": {"type": "commit", "sha": "old"}}, "owner/repo", "v1", "new", allow_missing=True)

    def test_annotated_tag_is_peeled(self):
        calls = []
        def request(url):
            calls.append(url)
            return {"object": {"type": "tag", "sha": "annotation"}} if len(calls) == 1 else {"object": {"type": "commit", "sha": "expected"}}
        publisher.require_matching_tag(request, "owner/repo", "v1", "expected", allow_missing=False)
        self.assertEqual(calls[1], "https://api.github.com/repos/owner/repo/git/tags/annotation")

    def test_missing_tag_only_allowed_before_publication(self):
        def missing(url):
            raise HTTPError(url, 404, "missing", {}, None)
        publisher.require_matching_tag(missing, "owner/repo", "v1", "expected", allow_missing=True)
        with self.assertRaises(HTTPError):
            publisher.require_matching_tag(missing, "owner/repo", "v1", "expected", allow_missing=False)

    def test_unknown_object_and_403_are_not_missing_tags(self):
        with self.assertRaises(ValueError):
            publisher.require_matching_tag(lambda _: {"object": {"type": "tree", "sha": "x"}}, "owner/repo", "v1", "x", allow_missing=True)
        def forbidden(url):
            raise HTTPError(url, 403, "forbidden", {}, None)
        with self.assertRaises(HTTPError):
            publisher.require_matching_tag(forbidden, "owner/repo", "v1", "x", allow_missing=True)


if __name__ == "__main__":
    unittest.main()
