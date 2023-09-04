# A parser and lexer for ical files
### Based on a talk about lexical scanning by Rob Pike
https://www.youtube.com/watch?v=HxaD_trXwRE&t=1271s

The lexer and the parser run concurrently. The lexer scanning runes in the input and returning tokens to the parser. The parser validates the ical tokens according to RFC5545.
