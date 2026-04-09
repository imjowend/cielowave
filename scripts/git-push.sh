#!/bin/bash
set -e

cd /vercel/share/v0-project

# Configure git
git config user.email "v0[bot]@users.noreply.github.com"
git config user.name "v0[bot]"

# Add changes
git add app/page.tsx next.config.mjs app/layout.tsx

# Commit
git commit -m "feat: implement Tidal OAuth integration and playlist generation

- Add main page with artist search and playlist generation
- Implement Tidal save flow with OAuth redirect
- Add query param handling for post-OAuth feedback
- Update next.config with callback rewrite
- Add Sonner toasts for user feedback

Co-authored-by: v0[bot] <v0[bot]@users.noreply.github.com>"

# Push to current branch
git push origin v0/jowend-aead8201

echo "✅ Changes pushed successfully!"
