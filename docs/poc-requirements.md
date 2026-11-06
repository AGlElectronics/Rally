# Rally — proof of concept requirements (draft)

Status: **MVP built** (phases 1–3), still open for discussion with the events
and website teams. The assumptions made while building are listed at the end.

Goal: give the events team enough to plan a convention in Rally instead of
Pretalx. In Pretalx the part that was really used was the submission form;
automatic scheduling was not used and is out of scope.

Nothing in Rally may hard-code a convention, organisation or domain. Every
name, host and form is configuration, so another convention can run the same
stack.

## Scope

| Phase | What | In the PoC |
| ----- | ---- | ---------- |
| 1 | Public submissions + staff review | yes |
| 2 | Schedule builder (drag and drop with rules) + public schedule API | yes |
| 3 | Live day: delays, cancellations, Telegram announcements | yes (MVP) |

## Who uses it

- **Attendee** — has an account on the convention website. Submits events and
  follows their status. Never sees the staff app.
- **Events lead** — sets up the call for papers and its form, decides on
  submissions, builds and publishes the schedule.
- **Reviewer** _(optional)_ — reads submissions and leaves notes or scores; the
  lead decides.
- **Admin** — configures Rally in Den (or the config file when standalone).

Lead and reviewer are **Den roles**, not Rally settings, because volunteers
join and leave all the time. Granting or removing them in Den (or through HR
onboarding/offboarding) changes access on the person's next sign-in or token
refresh; nothing to clean up in Rally. Standalone Rally reads the same role
names from the staff token.

## Two sites, two logins

- **Staff app** — today's Rally frontend (dev `events.conservices.test`,
  production e.g. `rally.<crew-domain>`). Staff sign in through the existing
  staff Keycloak realm.
- **Submission site** — a separate small public frontend on its own host
  (dev `submit.conservices.test`, production e.g. `events.<con-domain>`). The
  host is configuration, not code.
- **Attendees sign in with a separate, attendee-only Keycloak realm**, never
  the staff realm. In production that realm belongs to the website's account
  system; until the website team is ready we stub it with a dev realm
  (`attendees`, self-registration on). Same OIDC flow as staff today.
- The Rally backend trusts both issuers but keeps them apart: staff tokens
  only work on the staff API, attendee tokens only on the public API. User
  identity is already issuer + subject (ADR-0002), so one person in each realm
  is simply two users.

## Phase 1 — submissions

### Call for papers _(exists: title, description, draft/open/closed, dates)_

- One or more calls per edition (e.g. "Panels", "Stage shows").
- Opens and closes on its dates; can also be closed by hand.
- Each call has its **own form** (below) and a list of allowed **event types**.

### Configurable form

Set up by an events lead in Rally's settings; no field is hard-coded except
the few the schedule needs.

- Field types: short text, long text, number, duration, single choice,
  multiple choice, checkbox, email, URL, date/time, **file upload**.
- File fields set allowed types (e.g. images, PDF) and a max size.
- Per field: label, help text, required, options (for choices), min/max.
- **System fields** always present because scheduling depends on them:
  title, event type, requested duration, people involved (attendee accounts,
  see Co-presenters).
- Reorder fields; preview the form as an attendee sees it.
- A form can be copied from an earlier call or edition.
- Changing a form after submissions exist keeps old answers readable
  (answers are stored per field id; removed fields are shown as archived).

### Attendee side

- See open calls, fill in the form, save a draft, submit.
- See own submissions and their status; edit until the call closes; withdraw.
- Status wording is configurable (e.g. "In review", "Accepted").

### Co-presenters

Every person on a submission has an attendee account, so the schedule can
check "same person in two places" reliably. Linking them:

- **Invite link / code** (main way, works without sending email): adding a
  co-presenter creates a short code like `K7QM-3XPA` and a link. The
  co-presenter opens it while signed in and accepts; until then they show as
  "invited". The submitter can revoke or re-issue it.
- **Email match**: if the email typed in matches a _verified_ email on an
  attendee account, they are linked straight away (still shown to them so
  they can decline).
- **Presenter code**: each attendee has a personal code on their profile
  page on the submission site that they can give to whoever is adding them.
- A co-presenter sees the submission read-only, can leave it, and appears in
  the clash check once accepted.

### Uploads

- Files go **straight from the browser to object storage (S3)** with
  short-lived presigned upload URLs. The backend never stores or buffers the
  file on disk; it only issues the URL and records the object key, size and
  type after the upload completes.
- Downloads use short-lived presigned URLs, checked against who may see the
  submission.
- Uploads that were started but never completed or attached are removed by
  a background sweeper in the backend (works on any S3 provider, so no
  bucket lifecycle rule is needed).
- Dev uses MinIO (ADR-0004); production uses the real S3 provider.
- Email notifications: **not in the PoC**.

### Staff review

- List of submissions per call with filters (status, event type, search).
- Detail view with all answers and an internal notes thread (never shown to
  the attendee).
- **Review mode is a per-call setting** so it can change once the team
  decides:
  - _Lead decides_ (default): the lead accepts or rejects.
  - _Reviewers score, lead decides_: reviewers give a score + note, the lead
    sees the average and decides.
- Decisions are audited (who, when, note). _(accept/reject + note exists)_
- Accept/reject needs the `submission.decide` permission (events lead);
  reviewing and scoring need `submission.review`. Nobody scores or decides
  their own submission.
- The decision note is the message the submitter sees. Each call can rename
  the statuses the submitter sees (e.g. "Not accepted" instead of
  "Rejected").

## Phase 2 — schedule builder

### Setup

- **Date range** of the edition (days), with a default day start/end used
  only to size the grid.
- **Rooms / stages** (name, order, optional capacity).
- **Event types** (e.g. Panel, Stage show, Workshop): colour, default length,
  **required gap after** (changeover), and **which rooms they may use**.

### Building

- Grid: one column per room, one tab or band per day, snapping to 5/10/15 min
  (setting).
- Sidebar of accepted, unscheduled submissions; drag onto the grid; drag to
  move; resize to change length; drag back to unschedule.
- Staff can also add items that were never submitted (opening ceremony,
  breaks, staff-only blocks).

### Rules (checked live on every drop and on the server)

| Rule | Example |
| ---- | ------- |
| No overlap in a room | two items can't share Main Stage at 14:00 |
| Gap after an event type | after a stage show the room is blocked 30 min |
| Same person not in two places | a panelist can't be in two rooms at once (incl. gaps) |
| Event type allowed in room | stage shows only on rooms marked for them |

- Each rule is **block** or **warn** (setting per rule). Warnings show on the
  item and in a "conflicts" list; blocked drops snap back with the reason.
- Publishing is refused while blocking conflicts exist.

### Publishing

- Draft schedule vs **published** schedule: staff edit the draft, then
  publish. Published versions are kept (who, when) so a change can be
  compared or rolled back.

### Public schedule API

- Read-only JSON API the website consumes (days, rooms, items with times,
  type, description, people's public names, status). Optionally iCal.
- Only the **published** version, never the draft.
- **Can be switched on/off.** Public APIs are a per-module feature flag:
  driven by Den when Rally is paired (module settings in Den), or by Rally's
  config file / env when standalone. Off = endpoints return 404.
- Cache-friendly (ETag / short max-age) so the website can poll it.

## Phase 3 — live day

- Delay an item by N minutes, optionally pushing the rest of that room's day.
- Cancel / move an item; the public API reflects it immediately.
- Telegram bot: changes are drafted as announcements, a person confirms, the
  bot posts to one or more configured chats. Bot token stored as a secret
  (Den or config file).

## Configuration summary

| Setting | Where |
| ------- | ----- |
| Submission site host | config (compose / Caddy) _(Den setting later)_ |
| Attendee OIDC issuer + client | env `ATTENDEE_OIDC_ISSUER_URL` / `ATTENDEE_OIDC_CLIENT_ID` _(Den setting later)_ |
| Public schedule API on/off | Den module settings / config file |
| Calls, forms, event types, rooms, rules | Rally staff app (per edition) |
| Telegram bot token | env `TELEGRAM_BOT_TOKEN` (secret; empty = dry run) |
| Telegram chats | Rally staff app (per edition) |

## Open questions

1. Does the website need anything beyond the schedule from the public API
   (e.g. list of accepted events before the schedule exists)? _Left open until
   the next meeting with the website team._

## Phase 1 assumptions (made while building; easy to change)

- The submission site is the same frontend codebase built with
  `RALLY_APP=submit`, served on its own host.
- Attendee settings come from env for now; moving them into Den module
  settings is a later step.
- Closed calls stay publicly readable (so links keep working); only drafts
  are hidden.
- Linking a co-presenter by email only works once that person has signed in
  to the submission site with a verified email; otherwise the submitter gets
  an invite link to pass on.
- Removing a question from a form archives it, so existing answers are kept
  and still shown to staff.

## Phase 2 and 3 assumptions (made while building; easy to change)

- Permissions: `schedule.edit` builds the draft (rooms, items, settings,
  loading an old version); `schedule.publish` publishes, makes live-day
  changes and sends announcements. Organisation admins, edition organisers
  and programme managers get both by default.
- The grid and all times use the edition's timezone, whatever the browser's
  zone. A day on the grid is a local calendar day; an item can't run longer
  than 24 hours.
- Default rule modes: "person in two places" warns, the others block. The
  changeover gap counts for the same room and, for people, between rooms.
- A submission can be scheduled once, and only while it is accepted. If it is
  later un-accepted the item stays until someone removes it; loading an old
  version skips items whose submission is no longer accepted or whose room is
  gone.
- Live-day changes apply to the **published** schedule straight away (new
  version, kind "live") and are copied into the draft, so the next publish
  doesn't undo them. A live change that breaks a blocking rule asks first and
  can be applied anyway; publishing from the builder can't.
- "Push the rest of the room's day" moves every later item in that room on
  the same local day by the same amount.
- Each live change drafts an announcement; nothing is posted until someone
  presses Send. Without a bot token, sending records the message as a dry run.
- The public schedule is addressed by edition id, or by slug when only one
  edition with that slug has a published schedule. Staff-only items and
  account ids are never included. The Den key `events_public_schedule`
  (true/false) wins; otherwise `PUBLIC_SCHEDULE_ENABLED` decides.
- The submission site has a read-only "Programme" page using the same public
  API, with an "Add to calendar" link to the iCal feed.

## Decided

- Attendees use a separate attendee-only Keycloak realm, stubbed in dev the
  same way as the staff login. How the website's accounts feed that realm is
  the website team's side and doesn't change Rally.
- Events lead / reviewer are Den roles, changeable at any time.
- Co-presenters need attendee accounts, linked by invite code, verified email
  or presenter code.
- Forms accept uploads, stored directly in S3 via presigned URLs.
