Here is the cleaned-up documentation, organized into clear sections and updated to include the workflow for maintaining your documentation after the initial setup.

---

# Hosting Go Documentation with GitHub Pages

This guide explains how to generate Markdown documentation from your Go code using `gomarkdoc` and manually host it on a GitHub Pages (`gh-pages`) branch.

## 1. Initial Setup (One-Time)

Run these steps the very first time you want to create your documentation and the `gh-pages` branch.

**Step 1: Install gomarkdoc locally**

```bash
go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@latest

```

**Step 2: Generate the docs to a temporary folder**
Ensure you are on your `main` (or `master`) branch before running this:

```bash
mkdir -p /tmp/docs
gomarkdoc --output /tmp/docs/index.md ./...

```

**Step 3: Create the orphan `gh-pages` branch**
An orphan branch starts with a clean slate and no commit history.

```bash
git checkout --orphan gh-pages

```

**Step 4: Clear the working directory**
Remove all existing files from this new branch so it only contains your documentation.

```bash
git rm -rf .

```

**Step 5: Move the docs in, commit, and push**

```bash
cp -r /tmp/docs/* .
git add index.md
git commit -m "Initial docs deployment"
git push origin gh-pages

```

## 2. Enable GitHub Pages

Once your `gh-pages` branch is pushed, configure GitHub to serve your site:

1. Go to your repository on GitHub.
2. Click **Settings** > **Pages** (in the left sidebar).
3. Under **Source**, select **Deploy from a branch**.
4. Under **Branch**, select `gh-pages` and `/ (root)`.
5. Click **Save**.

*Your documentation will be live at `https://<username>.github.io/<repository-name>/` after a few minutes.*

## 3. Updating Documentation

Whenever you update your Go code and need to refresh the live documentation, follow this workflow:

**Step 1: Generate updated docs from your main branch**
Make sure you are on your `main` branch with your latest code.

```bash
git checkout main
gomarkdoc --output /tmp/docs/index.md ./...

```

**Step 2: Switch to the `gh-pages` branch**

```bash
git checkout gh-pages

```

**Step 3: Overwrite the old docs, commit, and push**

```bash
cp -r /tmp/docs/* .
git add index.md
git commit -m "Update documentation"
git push origin gh-pages

```

**Step 4: Return to your main branch**

```bash
git checkout main

```