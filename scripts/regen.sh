#!/bin/sh
# Regenerate the expected .html files in testdata/ from the reference
# implementation (Asciidoctor 2.0.26). Requires Ruby with the asciidoctor gem.
#
#   scripts/regen.sh                 # regenerate everything
#   scripts/regen.sh path/to/x.adoc  # regenerate selected fixtures
set -e
cd "$(dirname "$0")/.."
RUBY=${RUBY:-ruby}
if ! command -v "$RUBY" >/dev/null 2>&1 && [ -x /opt/rbenv/versions/3.3.6/bin/ruby ]; then
  RUBY=/opt/rbenv/versions/3.3.6/bin/ruby
fi
LANG=C.UTF-8 LC_ALL=C.UTF-8 TZ=UTC exec "$RUBY" scripts/regen.rb testdata "$@"
