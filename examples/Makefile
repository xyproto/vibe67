.PHONY: clean

all: hello ackermann factorial fib fib_bench hello_simple hello_test minimal primes sdl3example test_bool test_bool2 test_cli_args test_cstring_access test_fstring test_match

hello: add.c67 hello.c67
	../c67 -o $@ $^

ackermann: ackermann.c67
	../c67 -o $@ $<

factorial: factorial.c67
	../c67 -o $@ $<

fib: fib.c67
	../c67 -o $@ $<

fib_bench: fib_bench.c67
	../c67 -o $@ $<

hello_simple: hello_simple.c67
	../c67 -o $@ $<

hello_test: hello_test.c67
	../c67 -o $@ $<

minimal: minimal.c67
	../c67 -o $@ $<

primes: primes.c67
	../c67 -o $@ $<

sdl3example: sdl3example.c67
	../c67 -o $@ $<

sdl3example.exe: sdl3example.c67
	../c67 -o $@ $<

test_bool: test_bool.c67
	../c67 -o $@ $<

test_bool2: test_bool2.c67
	../c67 -o $@ $<

test_cli_args: test_cli_args.c67
	../c67 -o $@ $<

test_cstring_access: test_cstring_access.c67
	../c67 -o $@ $<

test_fstring: test_fstring.c67
	../c67 -o $@ $<

test_match: test_match.c67
	../c67 -o $@ $<

runsdl3: sdl3example
	./sdl3example

runsdl3win: sdl3example.exe
	wine sdl3example.exe

clean:
	rm -f hello ackermann factorial fib fib_bench hello_simple hello_test minimal primes sdl3example test_bool test_bool2 test_cli_args test_cstring_access test_fstring test_match sdl3example.exe
