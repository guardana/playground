# Example: a helpdesk assistant and payout changes

A team deploying the enforcer in front of its helpdesk assistant checks, before
production, that the assistant:

- may read customer records;
- changes a payout account only after an approval;
- sends no mail (the lab classes every `mail.send` as an untrusted external
  destination).

The directory is a workspace, laid out as
[docs/lab-files.md](../../docs/lab-files.md#a-workspace-outside-the-clone)
describes:

| File | What it is |
|---|---|
| `config/policies/helpdesk-assistant.json` | the team's policy |
| `config/gateway/scenarios/helpdesk-assistant.yaml` | its part of the enforcer's configuration: principal, agent, approvals |
| `config/approver/payouts-duty-lead.yaml` | who answers a held change, and how |
| `config/contracts/helpdesk-assistant.yaml` | the security contract the verifier grades the assistant's trace against |
| `trajectories/payouts-01-…yaml` | the calls: a read, a payout change sent until answered, a mail outside |
| `scenarios/payouts/payouts-01-…yaml` | what the trail, the victims' journals and the verifier must show |

## Run it

The lab refuses a workspace inside the clone, so copy it out first. From the
clone, after `make images` and `make lab-key` (the lab's top-level README):

    cp -R examples/helpdesk-payouts ~/helpdesk-lab
    LAB_WORKSPACE=~/helpdesk-lab make scenario ID=payouts-01-a-payout-change-waits-for-the-duty-lead

The last line is `pass` and the path of `report.md`.

## Fail it on purpose

Break the policy, never the expectation. In the copy, change
`payout-account-change-needs-a-person` from `REQUIRE_APPROVAL` to `ALLOW`: step
2 is decided `ALLOW`, step 3 opens a trail of its own, four trails open instead
of three, the CRM serves two changes, the approver answers nothing, and the
verifier exits 1 on `contract.helpdesk-assistant.payout-change-is-approved-first`.
Step 4 fails too, paired with the extra trail.

Change `no-mail-to-an-untrusted-destination` from `DENY` to `ALLOW` instead:
step 4 is decided `ALLOW`, the mail victim serves one, and the verifier reports
`contract.helpdesk-assistant.no-email`.
