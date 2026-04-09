#!/usr/bin/env python3
import subprocess
import os

os.chdir('/vercel/share/v0-project')

# Configure git
subprocess.run(['git', 'config', 'user.email', 'v0[bot]@users.noreply.github.com'], check=True)
subprocess.run(['git', 'config', 'user.name', 'v0[bot]'], check=True)

# Add changes
subprocess.run(['git', 'add', 'app/page.tsx', 'next.config.mjs', 'app/layout.tsx'], check=True)

# Commit
subprocess.run([
    'git', 'commit', '-m',
    '''feat: implement Tidal OAuth integration and playlist generation

- Add main page with artist search and playlist generation
- Implement Tidal save flow with OAuth redirect
- Add query param handling for post-OAuth feedback
- Update next.config with callback rewrite
- Add Sonner toasts for user feedback

Co-authored-by: v0[bot] <v0[bot]@users.noreply.github.com>'''
], check=True)

# Push to current branch
subprocess.run(['git', 'push', 'origin', 'v0/jowend-aead8201'], check=True)

print("✅ Changes pushed successfully to v0/jowend-aead8201!")
