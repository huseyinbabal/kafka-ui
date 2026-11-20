---
title: Live topology
description: Watch messages flow from producers through topics into consumer groups.
---

![Live topology](/kafka-ui/screenshots/live-topology.png)

The live view shows the whole cluster as a flow, updated every `collector.liveInterval` (default 2s):

- **Brokers** are the cards on top. Each shows its leader count, incoming message rate, and a **controller** badge for the active controller.
- **Topics** are the middle column. Bars show messages in and out per second, and badges show which brokers lead its partitions.
- **Consumer groups** are on the right. Each card has a colored border and text label for its lag status, the current lag and consume rate, and every consumer as `client@host`.

## Reading the flow

- **Arrows into topics:** the produce rate.
- **Curves from a topic to a group:** that group's consumption.
- **Line width and the speed of the moving dashes** grow with messages per second. Each line is labeled with its rate.
- **Idle links** are thin and still.

Producers can't be listed through the Kafka protocol, so they appear only as the incoming rate of each topic.

Group status:

| Status | Meaning |
|---|---|
| No lag | caught up |
| Behind | some lag |
| Far behind | 10,000 messages or more |
| Lag with no consumers | lagging, no members, and nothing consumed recently |

## Changes as they happen

When a consumer joins, it is marked **new** inside its group for a few seconds. Joins and leaves are listed under **Recent changes**. Scale a consumer deployment and watch the new members appear within a couple of seconds.

## Controls

- **Filter:** show a topic, group or consumer and its connections.
- **Hide internal topics:** on by default.
- **Pause:** freeze the view.
- **Details:** click any card for a details panel with a link to its page.

Rates come from the analytics collector (last minute), so a freshly opened view shows real numbers right away. The server builds the topology only while someone is watching, and the view stays smooth with 500+ nodes.
