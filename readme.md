# TeeworldsEconDiscordModerationBot

The TEDMB is a bot that connects to a Teeworlds server via its external console and writes the log into a dedicated Discord channel.
The basic workflow is, that the *administrator* of the bot creates a dedicated channel for this bot, preferrably only accessibly by the administrator and his/her moderators team.
After the channel has been created, the administrator adds the bot to the channel and starts monitoring a specic server by connecting the channel to a specific server.
This connection is established by the command `#moderate econIP:econPort` and cannot be terminated without restarting the bot itself, meaning this is a one time connection process.

Each `econIP:econPort` that is used by the admin in the `#moderate` command must be present in the configuration file `.env`
This way was chosen in order not to expose the econ password to the moderation staff, as they might gain too many access rights if they connected to the external console themselves.

## Requirements

- Needs the Go compiler in order to be compiled. That's all.

## Build

```shell
go get -d
go build .
```

## Example configuration

The configuration is done by creating the `.env` configuration file in the current working directory from where the executable is called.
This means that if you call the executable with `./TeeworldsEconDiscordModerationBot`, the `.env` file must be in the same directory as the executable.
Do you start the bot by using `cd ~/ && ~/TEDMB/./TeeworldsEconDiscordModerationBot`, your configuration file must be located in the home directory `~/`.

Log levels:
-0 :chat, teamchat, votes & rcon  # the recommended log level
-1: 0 & whisper                   # not recommended to eavesdrop on the conversations of others
-2: 1 & join & leave              # this gets spammy

```ini

# the discord developer api key is the token that needs to be put here
DISCORD_TOKEN=tyzxdcfkvbxckvbnmKCgkcGKCruasLVL

# the administrator is the only person allowed to actually  execute admin commands, 
# especially adding/removing moderators, connecting the bot to a specific discord channel, etc.
DISCORD_ADMIN="nickname#1234"

# it is possible give specific users moderation access.
# this can be used instead of having to manually add moerators to the list
# can be used to keep the moderator list the same after bot restarts
# moderators can be removed by the admin at runtime anyway.
DISCORD_MODERATORS="nickname#12345 nickname2#3456"

# this is the group that is pinged, when someone writes @mods, @mod, @admins, etc.
DISCORD_MODERATOR_ROLE="Server Moderator"

# this is a list of commands that can be used by moderators from the discord logging channels.
DISCORD_MODERATOR_COMMANDS="status vote say mute unmute mutes voteban unvoteban unvoteban_client votebans ban unban bans set_team"

# this is the list of possible servers that can be moderated.
# if the moderation bot is run on the same server as the Teeworlds servers,
# it is possible to use addresses like `localhost:9305` instead of passing the actual IP
# like `127.0.0.1:9305`
# Is the bot run from a different server, the external IPs of the Teeworlds servers are needed with their
# corresponding econ ports.
ECON_ADDRESSES=127.0.0.1:9303 127.0.0.1:9304 localhost:9305

# it is recommended to use a long password, either one econ password for all servers or 
# one password without any whitespace for each individual server.
ECON_PASSWORDS=abcdefghijklsgxdhgcfjhvgkjbhk.nrdxjcfhkjn

# the recommended logging level.
LOG_LEVEL=0
```

## Administrator commands

Administrator commands start with the prefix `#`.
They can only be executed by the administrator that has been defined in the `.env` file.

### moderate <IP:Port>

Starts the moderation of the server that exposes the external console at the address *<IP:Port>*  
The moderation cannot be stopped by any command.  
Also this command can only be executed once per server, thus limiting the logging of one Teeworlds server to one Discord channel.  

### add \<DiscordUsername#1234>

If the administrator of the bot did not add moderators, that are allowed to use the bot, to the moderators list by adding them in the `.env` file, the admin is able to manually add them this way, slowly granting them access to the bot.

### remove \<DiscordUsername#1234>

If some moderators should not have any access to the moderation bot, the admin is able to remove staff from the moderators list by executing this command.

### purge

Remove all moderators from the moderators list except for the admin that has been defined in the `.env`configuration file.

### clean *(Be Careful)*

Delete all messages that are within the channel.
Creates a new message in the channel and deletes all messages that are before that newly created message.

### spy \<nickname> *(concider people's privacy)*

If the log level is not being increased in order to see whisper messages, the administrator has the ability to enable spying of whisper messages sent by specific players.
This should be heavily evaluated before usage, thus the usage is only allowed by the owner/main administrator.

### unspy \<nickname>

Stop spying on a specific player's whisper messages.

### purgespy

Remove all spied on players from the spy list.


## Moderator commands

Moderators can execute Teeworlds server commands the same way they usually do in the remote console (rcon).
The only difference is, that they need to prefix those commands with a `?`

One special command is the `?help` command that shows all available commands that the moderators can use.

Examples:

```text
# ban a player
?ban 0 30 flaming

# send a server message
?say Server shutdown.

# move a player to the spectators
?set_team 13 -1
```

Any command specified in the `.env` file within `DISCORD_MODERATOR_COMMANDS` can be used by the administrator and the moderation staff.
It is possible to specify random commands that the Teeworlds server actually does not know.
This would lead to moderators being able to execute invalid commands that are not recognized by the Teeworlds server, making it pointless.

## Important Info

Important to know, imo.

### Logging of Discord Command Execution

In order for the server owner to ensure a proper logging of staff activity, on the server as well as for the commands that were executed in the respective Discord channels, each executed Discord command is being written with the server logs, by executing the Teeworlds' own `echo` command.
The resulting server log entry might look like this:

The server must have logs enabled for this to actually work.
This is done by defining a `logfile Server-5-` in the Teeworlds server configuration file.

### Discord Channel Log

The moderation bot ensures that the Discord message log is not older than 24 hours.
This is takes some load off of Discord and ensures some privacy for the users that play on the servers, as the moderation staff does and should not have an extended access to such information.

## Usage

- Start the bot
- Add your bot to a server/channel
- execute `#moderate 127.0.0.1:9303` in any channel that the bot has access to. That channel becommes the bot's log channel. Repeated executions of that command will not work, thus forcing you to restart the bot if a new channel should be used as the log channel.
- the bot tries to hide as much personal info, especially IPs, only in edge cases users see actual IPs.
- after the bot started, you can use commands like `?help`, `?status`, `?bans` and many more. The first three commands do not execute anything on the Teeworlds server, but this data is being evalued and memorized by the bot to ensure a smaller log size and less stress on the Teeworlds server.


  