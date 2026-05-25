---- MODULE PayoutFlow ----
EXTENDS Naturals

\* balances never go negative
NoNegativeBalance ==
    \A a \in Accounts: balance[a] >= 0

\* total issued value is conserved across transfers
ConservationOfValue ==
    Sum(balance) = TotalIssued

\* a helper operator, not an invariant
NextId == Cardinality(ids) + 1

AllInvariants ==
    /\ TypeOK
    /\ NoNegativeBalance
    /\ ConservationOfValue

====
