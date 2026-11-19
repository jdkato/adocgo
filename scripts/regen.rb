# Regenerates the expected HTML for every fixture under testdata/ using the
# real Asciidoctor command line implementation (in a single Ruby process).
#
# Usage: ruby scripts/regen.rb [testdata-dir] [file.adoc ...]
# Record the same home and checkout directories on every machine; the corpus
# test maps its own onto them (fixtureHome and fixtureRoot in corpus_test.go).
ENV['HOME'] = '/root'
FIXTURE_ROOT = '/home/user/adocgo'
require 'asciidoctor'
require 'asciidoctor/cli'
require 'stringio'
require 'shellwords'
require 'timeout'

ENV['SOURCE_DATE_EPOCH'] = '1700000000'
root = ARGV[0] || 'testdata'
files = ARGV.size > 1 ? ARGV[1..] : Dir.glob(File.join(root, '**', '*.adoc')).sort
files.reject! {|f| f.split('/').include? 'include' }
count = 0
files.each do |adoc|
  args = ['-q', '-o', '-']
  args << '-s' unless adoc.split('/').include? 'standalone'
  flags = adoc.sub(/\.adoc\z/, '.flags')
  args.concat Shellwords.split(File.read(flags)) if File.exist? flags
  args << adoc
  out = StringIO.new
  err = StringIO.new
  begin
    Timeout.timeout(60) do
      invoker = Asciidoctor::Cli::Invoker.new args
      invoker.redirect_streams out, err
      invoker.invoke!
    end
    # fixtures converted with a DocBook backend keep their output in NAME.xml
    bi = args.index '-b'
    ext = bi && args[bi + 1].to_s.start_with?('docbook') ? '.xml' : '.html'
    File.write adoc.sub(/\.adoc\z/, ext), out.string.gsub(Dir.pwd, FIXTURE_ROOT)
    count += 1
  rescue Exception => e # rubocop:disable Lint/RescueException
    warn "FAILED #{adoc}: #{e.class}: #{e.message.lines.first}"
  end
end
warn "generated #{count} of #{files.size} fixtures"
