import shutil
import os
import time
import subprocess
import urllib

class GitSupport:
    def __init__(self, repo_url, ref):
        self.repo_url = repo_url
        self.ref = ref

    @staticmethod
    def pkg_from_repo_url(repo_url: str) -> str:
        parsed = urllib.parse.urlparse(repo_url)
        path = parsed.path.rstrip("/")
        if path.endswith(".git"):
            path = path[: -len(".git")]
        path = path.lstrip("/")
        host = parsed.netloc or parsed.path.split("/")[0]
        if host and path and path.startswith(host):
            # Handles ssh-style git@github.com:org/repo.git
            path = "/".join(path.split("/")[1:])
        if host and path:
            return f"https://{host}/{path}"
        return path or repo_url

    @staticmethod
    def clone_repo(repo_url: str, ref: str | None = None) -> str:
        """
        Clone `repo_url` into a directory under the current working directory.
        Directory name: <cwd>/<repo-name>-<timestamp>
        Returns the destination path. On failure the created dir is removed and an error raised.
        """
        # Derive a friendly repo name (handles normal https/ssh/git URLs)
        parsed = urllib.parse.urlparse(repo_url)
        repo_name = ""
        if parsed.path:
            repo_name = os.path.splitext(os.path.basename(parsed.path.rstrip("/")))[0]
        else:
            # fallback for scp-like forms: git@github.com:org/repo.git
            tail = repo_url.split(":", 1)[-1]
            repo_name = os.path.splitext(os.path.basename(tail.rstrip("/")))[0]

        ts = int(time.time())
        dest = os.path.abspath(os.path.join(os.getcwd(), f"{repo_name}-{ts}"))

        try:
            subprocess.run(
                ["git", "clone", "--depth=1", repo_url, dest],
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            if ref:
                subprocess.run(
                    ["git", "-C", dest, "checkout", ref],
                    check=True,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                )
        except Exception as exc:
            if os.path.exists(dest):
                shutil.rmtree(dest, ignore_errors=True)
            raise RuntimeError(f"Error cloning repo '{repo_url}': {exc}") from exc

        return dest
