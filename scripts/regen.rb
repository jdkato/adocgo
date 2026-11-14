# Regenerates the expected HTML for every fixture under testdata/ using the
# real Asciidoctor command line implementation (in a single Ruby process).
#
# Usage: ruby scripts/regen.rb [testdata-dir] [file.adoc ...]
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
    File.write adoc.sub(/\.adoc\z/, '.html'), out.string
    count += 1
  rescue Exception => e # rubocop:disable Lint/RescueException
    warn "FAILED #{adoc}: #{e.class}: #{e.message.lines.first}"
  end
end
warn "generated #{count} of #{files.size} fixtures"
