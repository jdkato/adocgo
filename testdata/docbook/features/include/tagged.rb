# tag::all[]
require 'set'

# tag::greet[]
def greet name
  puts "Hello, #{name}!" # <1>
end
# end::greet[]

# tag::bye[]
def bye
  # tag::inner[]
  puts 'bye'
  # end::inner[]
end
# end::bye[]
# end::all[]
puts 'untagged'
