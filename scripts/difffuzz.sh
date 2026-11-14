#!/bin/sh
# Differential fuzzing against the reference implementation.
#
# Writes mutated variants of the corpus inputs to a scratch directory, renders
# them with Asciidoctor (requires Ruby with the asciidoctor gem) and then
# compares adocgo's output byte-for-byte.
#
#   scripts/difffuzz.sh [SEED] [FLAGS] [ROUNDS]
#   scripts/difffuzz.sh 7 "-a experimental -d book" 8
#
# Inputs on which Asciidoctor itself fails (empty expected output) show up as
# differences and can be ignored.
set -e
cd "$(dirname "$0")/.."
SEED=${1:-1}
FLAGS=$2
ROUNDS=${3:-4}
DIR=${DIFFFUZZ_DIR:-${TMPDIR:-/tmp}/adocgo-difffuzz}
RUBY=${RUBY:-ruby}
if ! command -v "$RUBY" >/dev/null 2>&1 && [ -x /opt/rbenv/versions/3.3.6/bin/ruby ]; then
  RUBY=/opt/rbenv/versions/3.3.6/bin/ruby
fi
rm -rf "$DIR"
mkdir -p "$DIR"
MUTDIR=$DIR MUTSEED=$SEED MUTROUNDS=$ROUNDS go test -count=1 -run TestMutDump . >/dev/null
if [ -n "$FLAGS" ]; then
  for f in "$DIR"/x/*.adoc; do echo "$FLAGS" > "${f%.adoc}.flags"; done
fi
LANG=C.UTF-8 LC_ALL=C.UTF-8 TZ=UTC "$RUBY" scripts/regen.rb "$DIR"
ADOCGO_CORPUS_DIR=$DIR go test -count=1 -v -run TestCorpus . | grep -a "output differs\|corpus:" || true
echo "fixtures are in $DIR"
