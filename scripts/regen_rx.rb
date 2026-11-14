# Records, for every regular expression used by adocgo, what Ruby's regexp
# engine matches on a set of sample inputs. The Go test TestRxCompat checks
# the bundled engine against these results.
#
# Usage: ruby scripts/regen_rx.rb testdata/rx-compat.json
require 'json'

path = ARGV[0] || 'testdata/rx-compat.json'
data = JSON.parse File.read(path, encoding: 'UTF-8')
inputs = data['inputs']
data['cases'].each do |c|
  opts = 0
  opts |= Regexp::MULTILINE if c['flags'].include? 'm'
  opts |= Regexp::IGNORECASE if c['flags'].include? 'i'
  rx = Regexp.new c['pattern'], opts
  c['results'] = inputs.map do |input|
    matches = []
    pos = 0
    while matches.size < 3 && pos <= input.length && (m = rx.match input, pos)
      matches << (0...m.size).map {|i| m.begin(i) ? "#{m.byteoffset(i)[0]}-#{m.byteoffset(i)[1]}" : 'n' }.join(',')
      pos = m.end(0) == m.begin(0) ? m.end(0) + 1 : m.end(0)
    end
    matches.join ';'
  end
end
File.write path, JSON.generate(data)
warn "recorded #{data['cases'].size} patterns x #{inputs.size} inputs"
