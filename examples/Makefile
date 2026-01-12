.PHONY: clean

all: hello ackermann factorial fib fib_bench hello_simple hello_test minimal primes sdl3example test_bool test_bool2 test_cli_args test_cstring_access test_fstring test_match

hello: add.v67 hello.v67
	../vibe67 -o $@ $^

ackermann: ackermann.v67
	../vibe67 -o $@ $<

factorial: factorial.v67
	../vibe67 -o $@ $<

fib: fib.v67
	../vibe67 -o $@ $<

fib_bench: fib_bench.v67
	../vibe67 -o $@ $<

hello_simple: hello_simple.v67
	../vibe67 -o $@ $<

hello_test: hello_test.v67
	../vibe67 -o $@ $<

minimal: minimal.v67
	../vibe67 -o $@ $<

primes: primes.v67
	../vibe67 -o $@ $<

sdl3example: sdl3example.v67
	../vibe67 -o $@ $<

sdl3example.exe: sdl3example.v67
	../vibe67 -o $@ $<

test_bool: test_bool.v67
	../vibe67 -o $@ $<

test_bool2: test_bool2.v67
	../vibe67 -o $@ $<

test_cli_args: test_cli_args.v67
	../vibe67 -o $@ $<

test_cstring_access: test_cstring_access.v67
	../vibe67 -o $@ $<

test_fstring: test_fstring.v67
	../vibe67 -o $@ $<

test_match: test_match.v67
	../vibe67 -o $@ $<

runsdl3: sdl3example
	./sdl3example

runsdl3win: sdl3example.exe
	wine sdl3example.exe

clean:
	rm -f hello ackermann factorial fib fib_bench hello_simple hello_test minimal primes sdl3example test_bool test_bool2 test_cli_args test_cstring_access test_fstring test_match sdl3example.exe
