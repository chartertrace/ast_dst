---- MODULE Good ----
EXTENDS Naturals

VARIABLE x

Init == x = 0
Next == x' = (x + 1) % 3
Inv  == x \in 0..2
====
