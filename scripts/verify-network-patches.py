#!/usr/bin/env python3
"""Verify the retained SDK snapshots against fixed upstream modules and patches.

Uses the current Go command and its checksum-verified module cache. Missing
modules are downloaded normally; no repository files are changed.
"""
import difflib
import json
import pathlib
import subprocess
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[1]


def main():
    entries = json.loads((ROOT / "third_party/network-patches.json").read_text())
    for entry in entries:
        selected = json.loads(subprocess.check_output(
            ["go", "list", "-m", "-json", entry["module"]], cwd=ROOT, text=True,
        ))
        assert selected["Version"] == entry["upstream_version"], entry["module"] + ": selected version differs"
        assert pathlib.Path(selected["Replace"]["Dir"]).resolve() == (ROOT / entry["replacement"]).resolve()
        # Download outside the root module so verification cannot repopulate
        # checksums removed by tidy for local replacements.
        with tempfile.TemporaryDirectory(prefix="categraf-sdk-verify-") as temporary:
            scratch = pathlib.Path(temporary)
            # Use the committed checksums in an isolated module. This verifies
            # cached/downloaded archives without requiring a second checksum
            # database lookup or adding replaced-module sums to the root.
            (scratch / "go.mod").write_text(
                "module categraf.local/sdk-verification\n\ngo 1.25.0\n\nrequire "
                + entry["module"] + " " + entry["upstream_version"] + "\n"
            )
            (scratch / "go.sum").write_text(
                entry["module"] + " " + entry["upstream_version"] + " " + entry["upstream_sum"] + "\n"
                + entry["module"] + " " + entry["upstream_version"] + "/go.mod " + entry["upstream_go_mod_sum"] + "\n"
            )
            result = subprocess.run(
                ["go", "mod", "download", "-json", entry["module"] + "@" + entry["upstream_version"]],
                cwd=temporary, text=True, capture_output=True, check=True,
            )
        metadata = json.loads(result.stdout)
        assert metadata["Sum"] == entry["upstream_sum"], entry["module"]
        assert metadata["GoModSum"] == entry["upstream_go_mod_sum"], entry["module"]
        upstream = pathlib.Path(metadata["Dir"])
        for package in entry.get("auxiliary_packages", []):
            sources = [p for p in (upstream / package).glob("*.go") if not p.name.endswith("_test.go")]
            assert sources, (entry["module"], package)
            for original in sources:
                retained_file = ROOT / entry["replacement"] / original.relative_to(upstream)
                assert retained_file.is_file(), str(retained_file) + ": dependency test helper is missing"

        retained = ROOT / entry["replacement"]
        delta = []
        changed = []
        for source in sorted(retained.rglob("*")):
            if not source.is_file() or source.name in ("go.mod", "go.sum"):
                continue
            relative = source.relative_to(retained)
            original = upstream / relative
            before = original.read_bytes() if original.exists() else b""
            after = source.read_bytes()
            if before == after:
                continue
            changed.append(str(relative))
            delta.extend(difflib.unified_diff(
                before.decode().splitlines(keepends=True),
                after.decode().splitlines(keepends=True),
                fromfile="a/" + str(relative), tofile="b/" + str(relative),
            ))
        assert changed == entry["modified_files"], entry["module"] + ": file list differs"
        assert "".join(delta) == (ROOT / entry["patch"]).read_text(), entry["module"] + ": patch differs"
        if (upstream / "go.mod").exists():
            assert (retained / "go.mod").read_bytes() == (upstream / "go.mod").read_bytes()
        else:
            assert (retained / "go.mod").read_text() == "module " + entry["module"] + "\n\ngo 1.25.0\n"
        if (upstream / "go.sum").exists():
            assert (retained / "go.sum").read_bytes() == (upstream / "go.sum").read_bytes()
        print(entry["module"], entry["upstream_version"], "OK")


if __name__ == "__main__":
    main()
