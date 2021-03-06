/*
   Copyright 2014 Outbrain Inc.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"github.com/outbrain/golib/log"
	"github.com/outbrain/orchestrator/app"
	"github.com/outbrain/orchestrator/config"
)

const prompt string = `
orchestrator [-c command] [-i instance] [--verbose|--debug] [... cli ] | http

Cheatsheet:
	Run orchestrator in HTTP mode:
	
	orchestrator --debug http
	
	For CLI executuon see details below.
	
-i (instance): 
	instance on which to operate, in "hostname" or "hostname:port" format.
	Default port is 3306 (or DefaultInstancePort in config)
	For some commands this argument can be ommitted altogether, and the
	value is implicitly the local hostname.
-s (Sibling/Subinstance/deStination)
	associated instance. Meaning depends on specific command.
	
-c (command):
	Listed below are all available commands; all of which apply for CLI execution (ignored by HTTP mode).
	Different flags are required for different commands; see specific documentation per commmand.

	Topology refactoring using classic MySQL replication commands
		(ie STOP SLAVE; START SLAVE UNTIL; CHANGE MASTER TO; ...)
		These commands require connected topology: slaves that are up and running; a lagging, stopped or 
		failed slave will disable use of most these commands. At least one, and typically two or more slaves 
		will be stopped for a short time during these operations. 
		
		move-up
			Move a slave one level up the topology; makes it replicate from its grandparent and become sibling of
			its parent. It is OK if the instance's master is not replicating. Examples:
			
			orchestrator -c move-up -i slave.to.move.up.com:3306

			orchestrator -c move-up
				-i not given, implicitly assumed local hostname
			
		move-up-slaves
			Moves slaves of the given instance one level up the topology, making them siblings of given instance.
			This is a (faster) shortcut to executing move-up on all slaves of given instance.
			Examples:
			
			orchestrator -c move-up-slaves -i slave.whose.subslaves.will.move.up.com[:3306]
			
			orchestrator -c move-up-slaves -i slave.whose.subslaves.will.move.up.com[:3306] --pattern=regexp.filter
				only apply to those instances that match given regex

		move-below
			Moves a slave beneath its sibling. Both slaves must be actively replicating from same master.
			The sibling will become instance's master. No action taken when sibling cannot act as master 
			(e.g. has no binary logs, is of incompatible version, incompatible binlog format etc.)
			Example:
			
			orchestrator -c move-below -i slave.to.move.com -s sibling.slave.under.which.to.move.com

			orchestrator -c move-below -s sibling.slave.under.which.to.move.com
				-i not given, implicitly assumed local hostname
			
		enslave-siblings
			Turn all siblings of a slave into its sub-slaves. No action taken for siblings that cannot become
			slaves of given instance (e.g. incompatible versions, binlog format etc.). This is a (faster) shortcut
			to executing move-below for all siblings of the given instance. Example:
			
			orchestrator -c enslave-siblings -i slave.whose.siblings.will.move.below.com
			
		repoint
			Make the given instance replicate from another instance without changing the binglog coordinates. There
			are little sanity checks to this and this is a risky operation. Use cases are: a rename of the master's 
			host, a corruption in relay-logs, move from beneath MaxScale & Binlog-server. Examples:
			
			orchestrator -c repoint -i slave.to.operate.on.com -s new.master.com
			
			orchestrator -c repoint -i slave.to.operate.on.com
				The above will repoint the slave back to its existing master without change 
			
			orchestrator -c repoint
				-i not given, implicitly assumed local hostname
			
		make-co-master
			Create a master-master replication. Given instance is a slave which replicates directly from a master.
			The master is then turned to be a slave of the instance. The master is expected to not be a slave.
			The read_only property of the slve is unaffected by this operation. Examples:
	
			orchestrator -c make-co-master -i slave.to.turn.into.co.master.com
			
			orchestrator -c make-co-master
				-i not given, implicitly assumed local hostname
				
		get-candidate-slave
			Information command suggesting the most up-to-date slave of a given instance, which can be promoted
			as local master to its siblings. If replication is up and running, this command merely gives an
			estimate, since slaves advance and progress continuously in different pace. If all slaves of given
			instance have broken replication (e.g. because given instance is dead), then this command provides
			with a definitve candidate, which could act as a replace master. See also regroup-slaves. Example:
			
			orchestrator -c get-candidate-slave -i instance.with.slaves.one.of.which.may.be.candidate.com
			

	Topology refactoring using Pseudo-GTID
		These operations require that the topology's master is periodically injected with pseudo-GTID,
		and that the PseudoGTIDPattern configuration is setup accordingly. Also consider setting 
		DetectPseudoGTIDQuery.
		Operations via Pseudo-GTID are typically slower, since they involve scanning of binary/relay logs.
		They impose less constraints on topology locations and affect less servers. Only servers that
		are being transported have their replication stopped. Their masters or destinations are unaffected.

		match-up
			Transport the slave one level up the hierarchy, making it child of its grandparent. This is
			similar in essence to move-up, only based on Pseudo-GTID. The master of the given instance 
			does not need to be alive or connected (and could in fact be crashed). It is never contacted.
			Grandparent instance must be alive and accessible.
			Examples:
			
			orchestrator -c match-up -i slave.to.match.up.com:3306

			orchestrator -c match-up
				-i not given, implicitly assumed local hostname
			
		match-up-slaves
			Matches slaves of the given instance one level up the topology, making them siblings of given instance.
			This is a (faster) shortcut to executing match-up on all slaves of given instance. The instance need
			not be alive / accessib;e / functional. It can be crashed.
			Example:
			
			orchestrator -c match-up-slaves -i slave.whose.subslaves.will.match.up.com

			orchestrator -c match-up-slaves -i slave.whose.subslaves.will.match.up.com[:3306] --pattern=regexp.filter
				only apply to those instances that match given regex

		match-below
			Matches a slave beneath another (destination) instance. The choice of destination is almost arbitrary;
			it must not be a child/descendant of the instance. But otherwise they don't have to be direct siblings,
			and in fact (if you know what you're doing), they don't actually have to belong to the same topology.
			The operation expects the transported instance to be "behind" the destination instance. It only finds out
			whether this is the case by the end; the operation is cancelled in the event this is not the case.
			No action taken when destination instance cannot act as master (e.g. has no binary logs, is of incompatible version, incompatible binlog format etc.)
			Examples:
			
			orchestrator -c match-below -i slave.to.transport.com -s instance.that.becomes.its.master

			orchestrator -c match-below -s destination.instance.that.becomes.its.master
				-i not given, implicitly assumed local hostname
			
		multi-match-slaves
			Matches all slaves of a given instance under another (destination) instance. This is a (faster) shortcut
			to matching said slaves one by one under the destination instance. In fact, this bulk operation is highly
			optimized and can execute in orders of magnitue faster, depeding on the nu,ber of slaves involved and their
			respective position behind the instance (the more slaves, the more savings).
			The instance itself may be crashed or inaccessible. It is not contacted throughout the operation. Examples:
			
			orchestrator -c multi-match-slaves -i instance.whose.slaves.will.transport -s instance.that.becomes.their.master
			
			orchestrator -c multi-match-slaves -i instance.whose.slaves.will.transport -s instance.that.becomes.their.master --pattern=regexp.filter
				only apply to those instances that match given regex
			
		rematch
			Reconnect a slave onto its master, via PSeudo-GTID. The use case for this operation is a non-crash-safe
			replication configuration (e.g. MySQL 5.5) with sync_binlog=1 and log_slave_updates. This operation
			implies crash-safe-replication and makes it possible for the slave to reconnect. Example:
			
			orchestrator -c rematch -i slave.to.rematch.under.its.master
			
		regroup-slaves
			Given an instance (possibly a crashed one; it is never being accessed), pick one of its slave and make it
			local master of its siblings, using Pseudo-GTID. It is uncertain that there *is* a slave that will be able to
			become master to all its siblings. But if there is one, orchestrator will pick such one. There are many
			constraints, most notably the replication positions of all slaves, whether they use log_slave_updates, and 
			otherwise version compatabilities etc.
			As many slaves that can be regrouped under promoted slves are operated on. The rest are untouched.
			This command is useful in the event of a crash. For example, in the event that a master dies, this operation
			can promote a candidate replacement and set up the remaining topology to correctly replicate from that
			replacement slave. Example:
			
			orchestrator -c regroup-slaves -i instance.with.slaves.one.of.which.will.turn.local.master.if.possible
			
			--debug is your friend.
			
		last-pseudo-gtid
			Information command; an authoritative way of detecting whether a Pseudo-GTID event exist for an instance,
			and if so, output the last Pseudo-GTID entry and its location. Example:
			
			orchestrator -c last-pseudo-gtid -i instance.with.possible.pseudo-gtid.injection

	General replication commands
		These commands issue various statements that relate to replication.
		stop-slave
			Issues a STOP SLAVE; command. Example:

			orchestrator -c stop-slave -i slave.to.be.stopped.com
			
		start-slave
			Issues a START SLAVE; command. Example:

			orchestrator -c start-slave -i slave.to.be.started.com
			
		skip-query
			On a failed replicating slave, skips a single query and attempts to resume replication.
			Only applies when the replication seems to be broken on SQL thread (e.g. on duplicate
			key error). Example:

			orchestrator -c skip-query -i slave.with.broken.sql.thread.com
			
		reset-slave
			Issues a RESET SLAVE command. Destructive to replication. Example:

			orchestrator -c reset-slave -i slave.to.reset.com
			
		detach-slave
			Stops replication and modified binlog position into an impossible, yet reversible, value.
			This effectively means the replication becomes broken. See reattach-slave. Example:
			
			orchestrator -c detach-slave -i slave.whose.replication.will.break.com
			
			Issuing this on an already detached slave will do nothing.
			
		reattach-slave
			Undo a detahc-slave operation. Reverses the binlog change into the original values, and 
			resumes replication. Example:
			
			orchestrator -c reattach-slave -i detahced.slave.whose.replication.will.amend.com

			Issuing this on an attached (i.e. normal) slave will do nothing.
	
		set-read-only
			Turn an instance read-only, via SET GLOBAL read_only := 1. Examples:
			
			orchestrator -c set-read-only -i instance.to.turn.read.only.com
			
			orchestrator -c set-read-only
				-i not given, implicitly assumed local hostname
			
		set-writeable
			Turn an instance writeable, via SET GLOBAL read_only := 0. Example:
			
			orchestrator -c set-writeable -i instance.to.turn.writeable.com
			
			orchestrator -c set-writeable
				-i not given, implicitly assumed local hostname
			
	
	Information commands
		These commands provide information about topologies, replication connections, or otherwise orchstrator's
		"inventory".
		
		find
			Find instances whose hostname matches given regex pattern. Example:
			
			orchestrator -c find -pattern "backup.*us-east"
			
		clusters
			List all clusters known to orchestrator. A cluster (aka topology, aka chain) is identified by its
			master (or one of its master if more than one exists). Example:
			
			orchesrtator -c clusters
				-i not given, implicitly assumed local hostname
			
		topology
			Show an ascii-graph of a replication topology, given a member of that topology. Example:
			
			orchestrator -c topology -i instance.belonging.to.a.topology.com
			
			orchestrator -c topology
				-i not given, implicitly assumed local hostname
			
			Instance must be already known to orchestrator. Topology is generated by orchestrator's mapping
			and not from synchronuous investigation of the instances. The generated topology may include
			instances that are dead, or whose replication is broken.
			
		which-instance
			Output the fully-qualified hostname:port representation of the given instance, or error if unknown
			to orchestrator. Examples:
			
			orchestrator -c which-instance -i instance.to.check.com
			
			orchestrator -c which-instance
				-i not given, implicitly assumed local hostname

		which-cluster
			Output the name of the cluster an instance belongs to, or error if unknown to orchestrator. Examples:
			
			orchestrator -c which-cluster -i instance.to.check.com
			
			orchestrator -c which-cluster
				-i not given, implicitly assumed local hostname

		which-cluster-instances
			Output the list of instances participating in same cluster as given instance; output is one line
			per instance, in hostname:port format. Examples:

			orchestrator -c which-cluster-instances -i instance.to.check.com
			
			orchestrator -c which-cluster-instances
				-i not given, implicitly assumed local hostname

		which-master
			Output the fully-qualified hostname:port representation of a given instance's master. Examples:
			
			orchestrator -c which-master -i a.known.slave.com
			
			orchestrator -c which-master
				-i not given, implicitly assumed local hostname
				
		which-slaves
			Output the fully-qualified hostname:port list of slaves (one per line) of a given instance (or empty
			list if	instance is not a master to anyone). Examples:
			 
			orchestrator -c which-slaves -i a.known.instance.com
			
			orchestrator -c which-slaves
				-i not given, implicitly assumed local hostname
				
		instance-status
			Output short status on a given instance (name, replication status, noteable configuration). Example2:
			
			orchestrator -c replication-status -i instance.to.investigate.com
			
			orchestrator -c replication-status
				-i not given, implicitly assumed local hostname

	Orchestrator instance management
		These command dig into the way orchestrator manages instances and operations on instances			
			
		discover
			Request that orchestrator cotacts given instance, reads its status, and upsert it into 
			orchestrator's respository. Examples: 
	
			orchestrator -c discover -i instance.to.discover.com:3306

			orchestrator -c discover -i cname.of.instance

			orchestrator -c discover
				-i not given, implicitly assumed local hostname
			
			Orchestrator will resolve CNAMEs and VIPs.

		forget
			Request that orchestrator removed given instance from its repository. If the instance is alive
			and connected through replication to otherwise known and live instances, orchestrator will
			re-discover it by nature of its discovery process. Instances are auto-removed via config's
			UnseenAgentForgetHours. If you happen to know a machine is decommisioned, for example, it 
			can be nice to remove it from the repository before it auto-expires. Example:  

			orchestrator -c forget -i instance.to.forget.com
			
			Orchestrator will *not* resolve CNAMEs and VIPs for given instance.
	
		begin-maintenance
			Request a maintenance lock on an instance. Topology changes require placing locks on the minimal set of
			affected instances, so as to avoid an incident of two uncoordinated operations on a smae instance (leading
			to possible chaos). Locks are placed in the backend database, and so multiple orchestrator instances are safe.
			Operations automatically acquire locks and release them. This command manually acquires a lock, and will
			block other operations on the instance until lock is released. 
			Note that orchestrator automatically assumed locks to be expired after MaintenanceExpireMinutes (in config).
			Example:
			
			orchestrator -c begin-maintenance -i instance.to.lock.com
			
		end-maintenance
			Remove maintenance lock; such lock may have been gained by an explicit begin-maintenance command implicitly
			by a topology change. You should generally only remove locks you have placed manually; orchestrator will 
			automatically expire locks after MaintenanceExpireMinutes (in config).
			Example:
			
			orchestrator -c end-maintenance -i locked.instance.com
	
	Crash recovery commands
	
		replication-analysis
			Request an analysis of potential crash incidents in all known topologies. 
			Output format is not yet stabilized and may change in the future. Do not trust the output
			for automated parsing. Use web API instead, at this time. Example:
			
			orchestrator -c replication-analysis
			
		recover
			Do auto-recovery given a dead instance. Orchestrator chooses the best course of action.
			The given instance must be acknowledged as dead and have slaves, or else there's nothing to do.
			--debug is your friend. Example:
			
			orchestrator -c recover -i dead.instance.com --debug
			
			
	Misc commands
	
		continuous
			Enter continuous mode, and actively poll for instances, diagnose problems, do maintenance etc.
			This type of work is typically done in HTTP mode. However nothing prevents orchestrator from
			doing it in command line. Invoking with "continuous" will run indefinitely. Example:
			
			orchestrator -c continuous  
			
		resolve
			Utility command to resolve a CNAME and return resolved hostname name. Example:
			
			orchestrator -c resolve -i cname.to.resolve
	`

// main is the application's entry point. It will either spawn a CLI or HTTP itnerfaces.
func main() {
	configFile := flag.String("config", "", "config file name")
	command := flag.String("c", "", "command (discover|forget|continuous|move-up|move-below|begin-maintenance|end-maintenance|clusters|topology)")
	strict := flag.Bool("strict", false, "strict mode (more checks, slower)")
	instance := flag.String("i", "", "instance, host:port")
	sibling := flag.String("s", "", "sibling instance, host:port")
	owner := flag.String("owner", "", "operation owner")
	reason := flag.String("reason", "", "operation reason")
	duration := flag.String("duration", "", "maintenance duration (format: 59s, 59m, 23h, 6d, 4w)")
	pattern := flag.String("pattern", "", "regular expression pattern")
	discovery := flag.Bool("discovery", true, "auto discovery mode")
	verbose := flag.Bool("verbose", false, "verbose")
	debug := flag.Bool("debug", false, "debug mode (very verbose)")
	stack := flag.Bool("stack", false, "add stack trace upon error")
	flag.Parse()

	log.SetLevel(log.ERROR)
	if *verbose {
		log.SetLevel(log.INFO)
	}
	if *debug {
		log.SetLevel(log.DEBUG)
	}
	if *stack {
		log.SetPrintStackTrace(*stack)
	}

	log.Info("starting")

	if len(*configFile) > 0 {
		config.ForceRead(*configFile)
	} else {
		config.Read("/etc/orchestrator.conf.json", "conf/orchestrator.conf.json", "orchestrator.conf.json")
	}
	if config.Config.Debug {
		log.SetLevel(log.DEBUG)
	}

	if len(flag.Args()) == 0 && *command == "" {
		// No command, no argument: just prompt
		fmt.Println(prompt)
		return
	}

	switch {
	case len(flag.Args()) == 0 || flag.Arg(0) == "cli":
		app.Cli(*command, *strict, *instance, *sibling, *owner, *reason, *duration, *pattern)
	case flag.Arg(0) == "http":
		app.Http(*discovery)
	default:
		log.Error("Usage: orchestrator --options... [cli|http]")
	}
}
