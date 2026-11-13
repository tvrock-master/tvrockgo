<template>

<div>
    <v-list dense>

    <div class="program-jump-buttons">
        <v-btn small depressed class="program-jump-button" @click="scrollToFeatureHeader('current')">放送中</v-btn>
        <v-btn small depressed class="program-jump-button" @click="scrollToFeatureHeader('schedule')">録画予約</v-btn>
        <v-btn small depressed class="program-jump-button" @click="scrollToFeatureHeader('new')">新番組</v-btn>
        <v-btn small depressed class="program-jump-button" @click="scrollToFeatureHeader('end')">最終回</v-btn>
        <v-btn small depressed class="program-jump-button" @click="scrollToFeatureHeader('first')">初回</v-btn>
        <v-btn small depressed class="program-jump-button" @click="scrollToFeatureHeader('free')">無料</v-btn>
        <v-btn small depressed class="program-jump-button" @click="bcastChange(label.TSP_TS)">地上波</v-btn>
        <v-btn small depressed class="program-jump-button" @click="bcastChange(label.TSP_BS)">BS</v-btn>
        <v-btn small depressed class="program-jump-button" @click="bcastChange(label.TSP_CS)">CS</v-btn>
        <v-btn small depressed class="program-jump-button" @click="bcastChange(label.TSP_OS)">ワンセグ</v-btn>
        <v-btn small depressed class="program-jump-button" @click="hourSel(12)">１２時</v-btn>
        <v-btn small depressed class="program-jump-button" @click="hourSel(22)">２２時</v-btn>
        <v-btn v-if="feature1.length > 0" small depressed class="program-jump-button" @click="scrollToFeatureHeader('feature1')">付録１</v-btn>
        <v-btn v-if="feature2.length > 0" small depressed class="program-jump-button" @click="scrollToFeatureHeader('feature2')">付録２</v-btn>
        <v-btn v-if="feature3.length > 0" small depressed class="program-jump-button" @click="scrollToFeatureHeader('feature3')">付録３</v-btn>
        <v-btn v-if="feature4.length > 0" small depressed class="program-jump-button" @click="scrollToFeatureHeader('feature4')">付録４</v-btn>
    </div>

    <v-divider></v-divider>

    <div class="program-section">
    <v-list-item ref="currentHeader" class="program-header">
        <v-list-item-content>
            <v-list-item-title class="white-text">放送中の番組</v-list-item-title>
            <!-- v-list-item-subtitle>新番組</v-list-item-subtitle -->
        </v-list-item-content>
    </v-list-item>

    <template v-for="item in currentprogram">
        <div :key="'cur_section_' + item.station" class="program-subsection">
        <v-subheader :key="'sub_cur_' + item.station" class="grey lighten-3 font-weight-bold program-subheader">{{ item.station }}</v-subheader>
        <v-list-item class="program-item" :key="`current_` + item.program.Sid + '_' + item.program.Eid + '_' + item.program.Start" @click="selectProgram(item.program)">
            <v-list-item-content>
                <v-list-item-title><v-icon v-if="item.program.IsRec" small color="red" class="mr-1" title="録画予約済み">mdi-clock-outline</v-icon>{{ item.program.StartD ? zpadSNM(item.program.StartD, true) : "--"}}～{{ item.program.EndD ? zpadSN(item.program.EndD, true) : "--" }}  {{ item.program.Title }}</v-list-item-title>
                <v-list-item-subtitle>{{ item.program.Subtitle }}</v-list-item-subtitle>
                <div class="program-actions">
                    <v-btn
                    v-if="!item.program.IsRec"
                    small
                    class="program-action-button program-action-button-record"
                    :elevation="1"
                    title="録画予約"
                    @click.stop="pgReserve(item.program)">
                        <v-icon small left>mdi-clock-outline</v-icon>録画
                    </v-btn>
                    <v-btn
                    v-else-if="item.program.StartD && item.program.EndD && item.program.StartD.getTime() <= nowTime() && nowTime() < item.program.EndD.getTime()"
                    small
                    class="program-action-button program-action-button-reserved"
                    :class="{ 'program-action-button-watch': item.program.IsWatch }"
                    :elevation="1"
                    title="録画停止"
                    @click.stop="pgStop(item.program.ID)">
                        <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.program.DeviceID || '予約' }}
                    </v-btn>
                    <v-btn
                    v-else-if="!item.program.Complete"
                    small
                    class="program-action-button program-action-button-reserved"
                    :class="{ 'program-action-button-watch': item.program.IsWatch }"
                    :elevation="1"
                    title="予約削除"
                    @click.stop="pgDelete(item.program.ID)">
                        <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.program.DeviceID || '予約' }}
                    </v-btn>
                    <v-btn
                    v-if="!item.program.IsRec && options && options.watchbutton"
                    small
                    class="program-action-button program-action-button-watch"
                    :elevation="1"
                    title="視聴予約"
                    @click.stop="pgReserve(item.program, true)">
                        <v-icon small left>mdi-television</v-icon>視聴
                    </v-btn>
                    <v-btn
                    small
                    class="program-action-button program-action-button-search"
                    :elevation="1"
                    title="検索"
                    @click.stop="pgSearch(item.program)">
                        <v-icon small left>mdi-archive-search-outline</v-icon>検索
                    </v-btn>
                </div>
            </v-list-item-content>
        </v-list-item>
        <v-divider :key="'div_cur_' + item.station"></v-divider>
        </div>

    </template>
    </div>

    <v-divider></v-divider>

    <div class="program-section">
    <v-list-item ref="scheduleHeader" class="program-header">
        <v-list-item-content>
            <v-list-item-title>録画予約</v-list-item-title>
        </v-list-item-content>
    </v-list-item>

    <v-divider></v-divider>

    <template v-for="item in recordingschedule">
        <!-- v-subheader :key="'sub_rs_' + item.station" class="grey lighten-3 font-weight-bold program-subheader">{{ item.station }}</v-subheader -->
        <v-list-item class="program-item" :key="`rs_` + item.sid + '_' + item.id" @click="selectProgram({Sid: item.sid, Eid: item.eid, Start: item.starttime, End: item.endtime})">
            <v-list-item-content>
                <v-list-item-title><v-icon small color="red" class="mr-1" title="録画予約済み">mdi-clock-outline</v-icon>{{ item.StartD ? zpadSNM(item.StartD, true) : "--"}}～{{ item.EndD ? zpadSN(item.EndD, true) : "--" }}  {{ item.title }}</v-list-item-title>
            </v-list-item-content>
        </v-list-item>
        <v-divider :key="'div_rs_' + item.id"></v-divider>
    </template>
    </div>

    <v-divider></v-divider>

    <div class="program-section">
    <v-list-item ref="newHeader" class="program-header">
        <v-list-item-content>
            <v-list-item-title>新番組</v-list-item-title>
            <!-- v-list-item-subtitle>新番組</v-list-item-subtitle -->
        </v-list-item-content>
    </v-list-item>

    <template v-for="group in newprogramGroups">
        <div :key="'new_section_' + group.name" class="program-subsection">
        <v-subheader :key="'sub_new_'+group.name" class="grey lighten-3 font-weight-bold program-subheader">{{ group.name }}</v-subheader>
        <v-list-item class="program-item" v-for="item in group.programs" :key="`new_` + item.Sid + '_' + item.Eid + '_' + item.Start" @click="selectProgram(item)">
            <v-list-item-content>
                <v-list-item-title><v-icon v-if="item.IsRec" small color="red" class="mr-1" title="録画予約済み">mdi-clock-outline</v-icon>{{ item.StartD ? zpadSNM(item.StartD, true) : "--" }}～{{ item.EndD ? zpadSN(item.EndD, true) :  "--" }}  {{ item.Title }}</v-list-item-title>
                <v-list-item-subtitle>{{ item.Subtitle }}</v-list-item-subtitle>
                <div class="program-actions">
                    <v-btn
                    v-if="!item.IsRec"
                    small
                    class="program-action-button program-action-button-record"
                    :elevation="1"
                    title="録画予約"
                    @click.stop="pgReserve(item)">
                        <v-icon small left>mdi-clock-outline</v-icon>録画
                    </v-btn>
                    <v-btn
                    v-else-if="item.StartD && item.EndD && item.StartD.getTime() <= nowTime() && nowTime() < item.EndD.getTime()"
                    small
                    class="program-action-button program-action-button-reserved"
                    :class="{ 'program-action-button-watch': item.IsWatch }"
                    :elevation="1"
                    title="録画停止"
                    @click.stop="pgStop(item.ID)">
                        <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                    </v-btn>
                    <v-btn
                    v-else-if="!item.Complete"
                    small
                    class="program-action-button program-action-button-reserved"
                    :class="{ 'program-action-button-watch': item.IsWatch }"
                    :elevation="1"
                    title="予約削除"
                    @click.stop="pgDelete(item.ID)">
                        <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                    </v-btn>
                    <v-btn
                    v-if="!item.IsRec && options && options.watchbutton"
                    small
                    class="program-action-button program-action-button-watch"
                    :elevation="1"
                    title="視聴予約"
                    @click.stop="pgReserve(item, true)">
                        <v-icon small left>mdi-television</v-icon>視聴
                    </v-btn>
                    <v-btn
                    small
                    class="program-action-button program-action-button-search"
                    :elevation="1"
                    title="検索"
                    @click.stop="pgSearch(item)">
                        <v-icon small left>mdi-archive-search-outline</v-icon>検索
                    </v-btn>
                </div>
            </v-list-item-content>

        </v-list-item>
        <v-divider :key="'div_new_' + group.name"></v-divider>
        </div>
    </template>
    </div>

    <v-divider></v-divider>

    <div class="program-section">
    <v-list-item ref="endHeader" class="program-header">
        <v-list-item-content>
            <v-list-item-title>最終回</v-list-item-title>
            <!-- v-list-item-subtitle>Logged In</v-list-item-subtitle -->
        </v-list-item-content>
    </v-list-item>

    <v-divider></v-divider>

    <template v-for="group in endprogramGroups">
        <div :key="'end_section_' + group.name" class="program-subsection">
        <v-subheader :key="'sub_end_' + group.name" class="grey lighten-3 font-weight-bold program-subheader">{{ group.name }}</v-subheader>
        <v-list-item class="program-item" v-for="item in group.programs" :key="`end_` + item.Sid + '_' + item.Eid + '_' + item.Start" @click="selectProgram(item)">
            <v-list-item-content>
                <v-list-item-title><v-icon v-if="item.IsRec" small color="red" class="mr-1" title="録画予約済み">mdi-clock-outline</v-icon>{{ item.StartD ? zpadSNM(item.StartD, true) : "--" }}～{{ item.EndD ? zpadSN(item.EndD, true) : "--" }}  {{ item.Title }}</v-list-item-title>
                <v-list-item-subtitle>{{ item.Subtitle }}</v-list-item-subtitle>
                <div class="program-actions">
                    <v-btn
                    v-if="!item.IsRec"
                    small
                    class="program-action-button program-action-button-record"
                    :elevation="1"
                    title="録画予約"
                    @click.stop="pgReserve(item)">
                        <v-icon small left>mdi-clock-outline</v-icon>録画
                    </v-btn>
                    <v-btn
                    v-else-if="item.StartD && item.EndD && item.StartD.getTime() <= nowTime() && nowTime() < item.EndD.getTime()"
                    small
                    class="program-action-button program-action-button-reserved"
                    :class="{ 'program-action-button-watch': item.IsWatch }"
                    :elevation="1"
                    title="録画停止"
                    @click.stop="pgStop(item.ID)">
                        <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                    </v-btn>
                    <v-btn
                    v-else-if="!item.Complete"
                    small
                    class="program-action-button program-action-button-reserved"
                    :class="{ 'program-action-button-watch': item.IsWatch }"
                    :elevation="1"
                    title="予約削除"
                    @click.stop="pgDelete(item.ID)">
                        <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                    </v-btn>
                    <v-btn
                    v-if="!item.IsRec && options && options.watchbutton"
                    small
                    class="program-action-button program-action-button-watch"
                    :elevation="1"
                    title="視聴予約"
                    @click.stop="pgReserve(item, true)">
                        <v-icon small left>mdi-television</v-icon>視聴
                    </v-btn>
                    <v-btn
                    small
                    class="program-action-button program-action-button-search"
                    :elevation="1"
                    title="検索"
                    @click.stop="pgSearch(item)">
                        <v-icon small left>mdi-archive-search-outline</v-icon>検索
                    </v-btn>
                </div>
            </v-list-item-content>
        </v-list-item>
        <v-divider :key="'div_end_' + group.name"></v-divider>
        </div>
    </template>
    </div>

    <v-divider></v-divider>

    <div class="program-section">
    <v-list-item ref="firstHeader" class="program-header">
        <v-list-item-content>
            <v-list-item-title>初回</v-list-item-title>
        </v-list-item-content>
    </v-list-item>

    <v-divider></v-divider>

    <template v-for="group in firstprogramGroups">
        <div :key="'first_section_' + group.name" class="program-subsection">
        <v-subheader :key="'first_new_' + group.name" class="grey lighten-3 font-weight-bold program-subheader">{{ group.name }}</v-subheader>
        <v-list-item class="program-item" v-for="item in group.programs" :key="`first_` + item.Sid + '_' + item.Eid + '_' + item.Start" @click="selectProgram(item)">
            <v-list-item-content>
                <v-list-item-title><v-icon v-if="item.IsRec" small color="red" class="mr-1" title="録画予約済み">mdi-clock-outline</v-icon>{{ item.StartD ? zpadSNM(item.StartD, true) : "--" }}～{{ item.EndD ? zpadSN(item.EndD, true) :  "--" }}  {{ item.Title }}</v-list-item-title>
                <v-list-item-subtitle>{{ item.Subtitle }}</v-list-item-subtitle>
                <div class="program-actions">
                    <v-btn
                    v-if="!item.IsRec"
                    small
                    class="program-action-button program-action-button-record"
                    :elevation="1"
                    title="録画予約"
                    @click.stop="pgReserve(item)">
                        <v-icon small left>mdi-clock-outline</v-icon>録画
                    </v-btn>
                    <v-btn
                    v-else-if="item.StartD && item.EndD && item.StartD.getTime() <= nowTime() && nowTime() < item.EndD.getTime()"
                    small
                    class="program-action-button program-action-button-reserved"
                    :class="{ 'program-action-button-watch': item.IsWatch }"
                    :elevation="1"
                    title="録画停止"
                    @click.stop="pgStop(item.ID)">
                        <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                    </v-btn>
                    <v-btn
                    v-else-if="!item.Complete"
                    small
                    class="program-action-button program-action-button-reserved"
                    :class="{ 'program-action-button-watch': item.IsWatch }"
                    :elevation="1"
                    title="予約削除"
                    @click.stop="pgDelete(item.ID)">
                        <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                    </v-btn>
                    <v-btn
                    v-if="!item.IsRec && options && options.watchbutton"
                    small
                    class="program-action-button program-action-button-watch"
                    :elevation="1"
                    title="視聴予約"
                    @click.stop="pgReserve(item, true)">
                        <v-icon small left>mdi-television</v-icon>視聴
                    </v-btn>
                    <v-btn
                    small
                    class="program-action-button program-action-button-search"
                    :elevation="1"
                    title="検索"
                    @click.stop="pgSearch(item)">
                        <v-icon small left>mdi-archive-search-outline</v-icon>検索
                    </v-btn>
                </div>
            </v-list-item-content>

        </v-list-item>
        <v-divider :key="'div_first_' + group.name"></v-divider>
        </div>
    </template>
    </div>

    <div class="program-section">
    <v-list-item ref="freeHeader" class="program-header">
        <v-list-item-content>
            <v-list-item-title>無料</v-list-item-title>
        </v-list-item-content>
    </v-list-item>

    <v-divider></v-divider>

    <v-list-item class="program-item" v-for="item in freeprogram" :key="`free_` + item.Sid + '_' + item.Eid + '_' + item.Start" @click="selectProgram(item)">
        <v-list-item-content>
            <v-list-item-title><v-icon v-if="item.IsRec" small color="red" class="mr-1" title="録画予約済み">mdi-clock-outline</v-icon>{{ item.StartD ? zpadSNM(item.StartD, true) : "--" }}～{{ item.EndD ? zpadSN(item.EndD, true) : "--" }}  {{ item.Title }}</v-list-item-title>
            <v-list-item-subtitle>{{ item.Subtitle }}</v-list-item-subtitle>
            <div class="program-actions">
                <v-btn
                v-if="!item.IsRec"
                small
                class="program-action-button program-action-button-record"
                :elevation="1"
                title="録画予約"
                @click.stop="pgReserve(item)">
                    <v-icon small left>mdi-clock-outline</v-icon>録画
                </v-btn>
                <v-btn
                v-else-if="item.StartD && item.EndD && item.StartD.getTime() <= nowTime() && nowTime() < item.EndD.getTime()"
                small
                class="program-action-button program-action-button-reserved"
                :class="{ 'program-action-button-watch': item.IsWatch }"
                :elevation="1"
                title="録画停止"
                @click.stop="pgStop(item.ID)">
                    <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                </v-btn>
                <v-btn
                v-else-if="!item.Complete"
                small
                class="program-action-button program-action-button-reserved"
                :class="{ 'program-action-button-watch': item.IsWatch }"
                :elevation="1"
                title="予約削除"
                @click.stop="pgDelete(item.ID)">
                    <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                </v-btn>
                <v-btn
                v-if="!item.IsRec && options && options.watchbutton"
                small
                class="program-action-button program-action-button-watch"
                :elevation="1"
                title="視聴予約"
                @click.stop="pgReserve(item, true)">
                    <v-icon small left>mdi-television</v-icon>視聴
                </v-btn>
                <v-btn
                small
                class="program-action-button program-action-button-search"
                :elevation="1"
                title="検索"
                @click.stop="pgSearch(item)">
                    <v-icon small left>mdi-archive-search-outline</v-icon>検索
                </v-btn>
            </div>
        </v-list-item-content>
    </v-list-item>
    </div>


    <div class="program-section">
    <v-list-item ref="feature1Header" class="program-header">
        <v-list-item-content>
            <v-list-item-title>付録１</v-list-item-title>
        </v-list-item-content>
    </v-list-item>

    <v-divider></v-divider>

    <v-list-item class="program-item" v-for="item in feature1" :key="`feature1_` + item.Sid + '_' + item.Eid + '_' + item.Start" @click="selectProgram(item)">
        <v-list-item-content>
            <v-list-item-title><v-icon v-if="item.IsRec" small color="red" class="mr-1" title="録画予約済み">mdi-clock-outline</v-icon>{{ item.StartD ? zpadSNM(item.StartD, true) : "--" }}～{{ item.EndD ? zpadSN(item.EndD, true) : "--" }}  {{ item.Title }}</v-list-item-title>
            <v-list-item-subtitle>{{ item.Subtitle }}</v-list-item-subtitle>
            <div class="program-actions">
                <v-btn
                v-if="!item.IsRec"
                small
                class="program-action-button program-action-button-record"
                :elevation="1"
                title="録画予約"
                @click.stop="pgReserve(item)">
                    <v-icon small left>mdi-clock-outline</v-icon>録画
                </v-btn>
                <v-btn
                v-else-if="item.StartD && item.EndD && item.StartD.getTime() <= nowTime() && nowTime() < item.EndD.getTime()"
                small
                class="program-action-button program-action-button-reserved"
                :class="{ 'program-action-button-watch': item.IsWatch }"
                :elevation="1"
                title="録画停止"
                @click.stop="pgStop(item.ID)">
                    <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                </v-btn>
                <v-btn
                v-else-if="!item.Complete"
                small
                class="program-action-button program-action-button-reserved"
                :class="{ 'program-action-button-watch': item.IsWatch }"
                :elevation="1"
                title="予約削除"
                @click.stop="pgDelete(item.ID)">
                    <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                </v-btn>
                <v-btn
                v-if="!item.IsRec && options && options.watchbutton"
                small
                class="program-action-button program-action-button-watch"
                :elevation="1"
                title="視聴予約"
                @click.stop="pgReserve(item, true)">
                    <v-icon small left>mdi-television</v-icon>視聴
                </v-btn>
                <v-btn
                small
                class="program-action-button program-action-button-search"
                :elevation="1"
                title="検索"
                @click.stop="pgSearch(item)">
                    <v-icon small left>mdi-archive-search-outline</v-icon>検索
                </v-btn>
            </div>
        </v-list-item-content>
    </v-list-item>
    </div>


    <div class="program-section">
    <v-list-item ref="feature2Header" class="program-header">
        <v-list-item-content>
            <v-list-item-title>付録２</v-list-item-title>
        </v-list-item-content>
    </v-list-item>

    <v-divider></v-divider>

    <v-list-item class="program-item" v-for="item in feature2" :key="`feature2_` + item.Sid + '_' + item.Eid + '_' + item.Start" @click="selectProgram(item)">
        <v-list-item-content>
            <v-list-item-title><v-icon v-if="item.IsRec" small color="red" class="mr-1" title="録画予約済み">mdi-clock-outline</v-icon>{{ item.StartD ? zpadSNM(item.StartD, true) : "--" }}～{{ item.EndD ? zpadSN(item.EndD, true) : "--" }}  {{ item.Title }}</v-list-item-title>
            <v-list-item-subtitle>{{ item.Subtitle }}</v-list-item-subtitle>
            <div class="program-actions">
                <v-btn
                v-if="!item.IsRec"
                small
                class="program-action-button program-action-button-record"
                :elevation="1"
                title="録画予約"
                @click.stop="pgReserve(item)">
                    <v-icon small left>mdi-clock-outline</v-icon>録画
                </v-btn>
                <v-btn
                v-else-if="item.StartD && item.EndD && item.StartD.getTime() <= nowTime() && nowTime() < item.EndD.getTime()"
                small
                class="program-action-button program-action-button-reserved"
                :class="{ 'program-action-button-watch': item.IsWatch }"
                :elevation="1"
                title="録画停止"
                @click.stop="pgStop(item.ID)">
                    <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                </v-btn>
                <v-btn
                v-else-if="!item.Complete"
                small
                class="program-action-button program-action-button-reserved"
                :class="{ 'program-action-button-watch': item.IsWatch }"
                :elevation="1"
                title="予約削除"
                @click.stop="pgDelete(item.ID)">
                    <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                </v-btn>
                <v-btn
                v-if="!item.IsRec && options && options.watchbutton"
                small
                class="program-action-button program-action-button-watch"
                :elevation="1"
                title="視聴予約"
                @click.stop="pgReserve(item, true)">
                    <v-icon small left>mdi-television</v-icon>視聴
                </v-btn>
                <v-btn
                small
                class="program-action-button program-action-button-search"
                :elevation="1"
                title="検索"
                @click.stop="pgSearch(item)">
                    <v-icon small left>mdi-archive-search-outline</v-icon>検索
                </v-btn>
            </div>
        </v-list-item-content>
    </v-list-item>
    </div>

    <div class="program-section">
    <v-list-item ref="feature3Header" class="program-header">
        <v-list-item-content>
            <v-list-item-title>付録３</v-list-item-title>
        </v-list-item-content>
    </v-list-item>

    <v-divider></v-divider>

    <v-list-item class="program-item" v-for="item in feature3" :key="`feature3_` + item.Sid + '_' + item.Eid + '_' + item.Start" @click="selectProgram(item)">
        <v-list-item-content>
            <v-list-item-title><v-icon v-if="item.IsRec" small color="red" class="mr-1" title="録画予約済み">mdi-clock-outline</v-icon>{{ item.StartD ? zpadSNM(item.StartD, true) : "--" }}～{{ item.EndD ? zpadSN(item.EndD, true) : "--" }}  {{ item.Title }}</v-list-item-title>
            <v-list-item-subtitle>{{ item.Subtitle }}</v-list-item-subtitle>
            <div class="program-actions">
                <v-btn
                v-if="!item.IsRec"
                small
                class="program-action-button program-action-button-record"
                :elevation="1"
                title="録画予約"
                @click.stop="pgReserve(item)">
                    <v-icon small left>mdi-clock-outline</v-icon>録画
                </v-btn>
                <v-btn
                v-else-if="item.StartD && item.EndD && item.StartD.getTime() <= nowTime() && nowTime() < item.EndD.getTime()"
                small
                class="program-action-button program-action-button-reserved"
                :class="{ 'program-action-button-watch': item.IsWatch }"
                :elevation="1"
                title="録画停止"
                @click.stop="pgStop(item.ID)">
                    <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                </v-btn>
                <v-btn
                v-else-if="!item.Complete"
                small
                class="program-action-button program-action-button-reserved"
                :class="{ 'program-action-button-watch': item.IsWatch }"
                :elevation="1"
                title="予約削除"
                @click.stop="pgDelete(item.ID)">
                    <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                </v-btn>
                <v-btn
                v-if="!item.IsRec && options && options.watchbutton"
                small
                class="program-action-button program-action-button-watch"
                :elevation="1"
                title="視聴予約"
                @click.stop="pgReserve(item, true)">
                    <v-icon small left>mdi-television</v-icon>視聴
                </v-btn>
                <v-btn
                small
                class="program-action-button program-action-button-search"
                :elevation="1"
                title="検索"
                @click.stop="pgSearch(item)">
                    <v-icon small left>mdi-archive-search-outline</v-icon>検索
                </v-btn>
            </div>
        </v-list-item-content>
    </v-list-item>
    </div>


    <div class="program-section">
    <v-list-item ref="feature4Header" class="program-header">
        <v-list-item-content>
            <v-list-item-title>付録４</v-list-item-title>
        </v-list-item-content>
    </v-list-item>

    <v-divider></v-divider>

    <v-list-item class="program-item" v-for="item in feature4" :key="`feature4_` + item.Sid + '_' + item.Eid + '_' + item.Start" @click="selectProgram(item)">
        <v-list-item-content>
            <v-list-item-title><v-icon v-if="item.IsRec" small color="red" class="mr-1" title="録画予約済み">mdi-clock-outline</v-icon>{{ item.StartD ? zpadSNM(item.StartD, true) : "--" }}～{{ item.EndD ? zpadSN(item.EndD, true) : "--" }}  {{ item.Title }}</v-list-item-title>
            <v-list-item-subtitle>{{ item.Subtitle }}</v-list-item-subtitle>
            <div class="program-actions">
                <v-btn
                v-if="!item.IsRec"
                small
                class="program-action-button program-action-button-record"
                :elevation="1"
                title="録画予約"
                @click.stop="pgReserve(item)">
                    <v-icon small left>mdi-clock-outline</v-icon>録画
                </v-btn>
                <v-btn
                v-else-if="item.StartD && item.EndD && item.StartD.getTime() <= nowTime() && nowTime() < item.EndD.getTime()"
                small
                class="program-action-button program-action-button-reserved"
                :class="{ 'program-action-button-watch': item.IsWatch }"
                :elevation="1"
                title="録画停止"
                @click.stop="pgStop(item.ID)">
                    <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                </v-btn>
                <v-btn
                v-else-if="!item.Complete"
                small
                class="program-action-button program-action-button-reserved"
                :class="{ 'program-action-button-watch': item.IsWatch }"
                :elevation="1"
                title="予約削除"
                @click.stop="pgDelete(item.ID)">
                    <v-icon small left color="red">mdi-clock-outline</v-icon>{{ item.DeviceID || '予約' }}
                </v-btn>
                <v-btn
                v-if="!item.IsRec && options && options.watchbutton"
                small
                class="program-action-button program-action-button-watch"
                :elevation="1"
                title="視聴予約"
                @click.stop="pgReserve(item, true)">
                    <v-icon small left>mdi-television</v-icon>視聴
                </v-btn>
                <v-btn
                small
                class="program-action-button program-action-button-search"
                :elevation="1"
                title="検索"
                @click.stop="pgSearch(item)">
                    <v-icon small left>mdi-archive-search-outline</v-icon>検索
                </v-btn>
            </div>
        </v-list-item-content>
    </v-list-item>
    </div>


    </v-list>

</div>

</template>

<script>
import label from '../define.js';
import utility from '../utility.js';
import axios from 'axios';

export default {
  name: 'SideNavigation',
  props: [
    'api',
    'schedule',
    'timetable',
    'feature',
    'station',
    'options',
  ],

  data() {
    return {
        label: null,
        currentprogram: [],
        recordingschedule: [],
        newprogram: [],
        endprogram: [],
        firstprogram: [],
        freeprogram: [],
        feature1: [],
        feature2: [],
        feature3: [],
        feature4: [],
        stickyResizeObserver: null,
        genres: [
            { name: "ニュース／報道", code: 0 },
            { name: "スポーツ", code: 1 },
            { name: "情報／ワイドショー", code: 2 },
            { name: "ドラマ", code: 3 },
            { name: "音楽", code: 4 },
            { name: "バラエティ", code: 5 },
            { name: "映画", code: 6 },
            { name: "アニメ／特撮", code: 7 },
            { name: "ドキュメンタリー／教養", code: 8 },
            { name: "劇場／公演", code: 9 },
            { name: "趣味／教育", code: 10 },
            { name: "拡張（拡張用フラグ）／予備", code: 11 },
            { name: "その他", code: -1 }
        ],
        nowCache: null,
    };
  },

  computed: {
    newprogramGroups() {
      return this.getGroups(this.newprogram);
    },
    endprogramGroups() {
      return this.getGroups(this.endprogram);
    },
    firstprogramGroups() {
      return this.getGroups(this.firstprogram);
    }
  },

  methods: {

    withRecordingState(program) {
        const recording = (this.schedule || []).find(s =>
            s.validate && s.sid == program.Sid && s.eid == program.Eid);
        return {
            ...program,
            IsRec: !!recording,
            IsWatch: !!(recording && recording.watchonly && !recording.recordonly),
            ID: recording ? recording.id : 0,
            DeviceID: recording ? recording.deviceid : '',
            Complete: recording ? recording.complete : false,
        };
    },

    refreshPrograms() {
        if (this.timetable) {
            this.renderNew(this.timetable.services, this.timetable);
        }
    },

    getGroups(programs) {
      const groups = [];
      this.genres.forEach(genre => {
        const filtered = programs.filter(item => {
          if (genre.code === -1) {
            return item.Cnib_a === undefined || item.Cnib_a < 0 || item.Cnib_a > 11;
          }
          return item.Cnib_a === genre.code;
        });
        if (filtered.length > 0) {
          groups.push({
            name: genre.name,
            programs: filtered
          });
        }
      });
      return groups;
    },

    scrollToFeatureHeader(name) {
        this.updateStickyOffsets();
        let header = this.$refs[name + 'Header'];
        if (Array.isArray(header)) {
            header = header[0];
        }
        const target = header && (header.$el || header);
        if (!target) {
            return;
        }

        const container = this.getScrollContainer(target);
        if (!container) {
            target.scrollIntoView({ behavior: 'smooth', block: 'start' });
            return;
        }

        const anchor = target.closest('.program-section') || target;
        const offset = this.getStickyPixelValue('--program-jump-offset');
        const containerRect = container.getBoundingClientRect();
        const targetRect = anchor.getBoundingClientRect();
        container.scrollTo({
            top: container.scrollTop + targetRect.top - containerRect.top - offset,
            behavior: 'smooth'
        });
    },

    bcastChange(b) {
        this.$emit('bcastchange', b);
    },

    hourSel(b) {
        this.$emit('hourchange', b);
    },

    getStickyPixelValue(name) {
        if (!this.$el) {
            return 0;
        }
        const value = window.getComputedStyle(this.$el).getPropertyValue(name);
        const parsed = parseFloat(value);
        return Number.isNaN(parsed) ? 0 : parsed;
    },

    updateStickyOffsets() {
        if (!this.$el) {
            return;
        }
        const jumpButtons = this.$el.querySelector('.program-jump-buttons');
        const programHeader = this.$el.querySelector('.program-header');
        const jumpOffset = jumpButtons ? jumpButtons.offsetHeight : 0;
        const headerOffset = programHeader ? programHeader.offsetHeight : 0;
        this.$el.style.setProperty('--program-jump-offset', jumpOffset + 'px');
        this.$el.style.setProperty('--program-subheader-offset', (jumpOffset + headerOffset) + 'px');
    },

    getScrollContainer(el) {
        let parent = el.parentElement;
        while (parent && parent !== document.body) {
            const style = window.getComputedStyle(parent);
            if ((style.overflowY === 'auto' || style.overflowY === 'scroll') && parent.scrollHeight > parent.clientHeight) {
                return parent;
            }
            parent = parent.parentElement;
        }
        return document.scrollingElement || document.documentElement;
    },

    renderNew(so, dt) {
        if (!so || !dt) {
            return;
        }

        let cp = [];
        let rs = [];
        let np = [];
        let ep = [];
        let ip = [];
        let fp = [];
        let f1 = [];
        let f2 = [];
        let f3 = [];
        let f4 = [];

        let now = this.nowTime();
        let nowm = now / label.MSEC
        so.forEach(v => {
            let list = dt.Tvp[v];
            if (!list) {
                return;
            }
            list.some( prg => {
                if (prg.StartD && prg.EndD && prg.StartD.getTime() <= now && now < prg.EndD.getTime()) {
                    cp.push({
                        station: dt.Sta[v].Name + '(' + dt.Sta[v].Sid + ')',
                        program: this.withRecordingState(prg),
                    });
                    return true;
                }
            });
        });

        let n = 0;
        for (const v of this.schedule || []) {
            if (n>=10)
                break;
            if (v.complete || (v.endtime+v.tailtime) <= nowm)
                continue;
            rs.push(v);
            n++;
        }

        if (this.feature) {
            let bc = this.options.broadcast;
            if (bc >= label.TSP_TS && bc <= label.TSP_OS) {
                let feature = this.feature[bc-1];
                np = feature["new"] || [];
                ep = feature["end"] || [];
                ip = feature["first"] || [];
                fp = feature["free"] || [];
                f1 = feature["feature1"] || [];
                f2 = feature["feature2"] || [];
                f3 = feature["feature3"] || [];
                f4 = feature["feature4"] || [];
            }
        }

        this.currentprogram = cp;
        this.recordingschedule = rs;
        this.newprogram = np.map(this.withRecordingState);
        this.endprogram = ep.map(this.withRecordingState);
        this.firstprogram = ip.map(this.withRecordingState);
        this.freeprogram = fp.map(this.withRecordingState);
        this.feature1 = f1.map(this.withRecordingState);
        this.feature2 = f2.map(this.withRecordingState);
        this.feature3 = f3.map(this.withRecordingState);
        this.feature4 = f4.map(this.withRecordingState);
        this.nowCache = utility.now();
    }, 


    zpadSN(t, te) {
        let h = t.getHours();
        return (te && h < 4 ? h+24 : h) + ':' + ('00' + t.getMinutes()).slice(-2);
    },

    zpadSNM(t, te) {
        if (!this.nowCache) {
            this.nowCache = utility.now();
        }
        let now = this.nowCache;
        let date = '';
        if (t.getMonth() != now.getMonth() || t.getDate() != now.getDate()) {
            date = '' + (t.getMonth()+1) + '/' + t.getDate() + ' ';
        }
        let h = t.getHours();
        return date + (te && h < 4 ? h+24 : h) + ':' + ('00' + t.getMinutes()).slice(-2);
    },

    nowTime() {
        return utility.now().getTime();
    },

    pgReserve(item, watch = false) {
        let par = {
            sid: item.Sid,
            eid: item.Eid,
            start: item.Start,
            end: item.End
        };
        if (watch) par.watch = true;
        axios.post(this.api.schedule, JSON.stringify(par)).then(res => {
            utility.log("api schedule results:"+res.status);
        }).catch(err => {
            utility.err(`request schedule error: ${err}`);
        });
    },

    pgSearch(item) {
        this.$emit('programsearch', {
            title: utility.extractUnifiedTitle(item.Title),
            ssid: item.Sid,
        });
    },

    pgDelete(id) {
        let sch = this.schedule || [];

        for (let i = 0; i < sch.length; i++) {
            if (id == sch[i].id) {
                // searchidがあれば無効にする
                if (sch[i].searchid>0) {
                    let val = sch[i];
                    val.validate = !val.validate;
                    axios.put(this.api.schedule, JSON.stringify(val)).then(res => {
                        utility.log("api schedule results:"+res.status);
                    }).catch(err => {
                        utility.err(`request schedule error: ${err}`);
                    });
                } else {
                    axios.delete(this.api.schedule+'/'+sch[i].id).then(res => {
                        utility.log("api schedule results:"+res.status);
                    }).catch(err => {
                        utility.err(`request delete schedule error: ${err}`);
                    });
                }
                return;
            }
        }
        utility.err("schedule not found ("+id+")")
    },

    pgStop(id) {
        let sch = this.schedule || [];

        for (let i = 0; i < sch.length; i++) {
            if (id == sch[i].id) {
                axios.delete(this.api.schedule+'/stop/'+sch[i].id).then(res => {
                    utility.log("api schedule results:"+res.status);
                }).catch(err => {
                    utility.err(`request stop schedule error: ${err}`);
                });
                return;
            }
        }
        utility.err("schedule not found ("+id+")")
    },

    selectProgram(item) {
        this.$emit('select', {
            sid: item.Sid,
            eid: item.Eid,
            starttime: item.Start,
            endtime: item.End,
        });
    }

  },

  watch: {
    timetable: {
        immediate: true,
        handler: 'refreshPrograms',
    },
    feature: 'refreshPrograms',
    schedule: {
        deep: true,
        handler: 'refreshPrograms',
    },
    'options.broadcast': 'refreshPrograms',
  },

  created() {
      this.label = label;
  },
  mounted() {
    this.$nextTick(this.updateStickyOffsets);
    if (typeof ResizeObserver !== 'undefined') {
        this.stickyResizeObserver = new ResizeObserver(this.updateStickyOffsets);
        this.stickyResizeObserver.observe(this.$el);
    } else {
        window.addEventListener('resize', this.updateStickyOffsets);
    }
  },
  updated() {
    this.$nextTick(this.updateStickyOffsets);
  },
  destroyed() {
    if (this.stickyResizeObserver) {
        this.stickyResizeObserver.disconnect();
        this.stickyResizeObserver = null;
    } else {
        window.removeEventListener('resize', this.updateStickyOffsets);
    }
  }
};
</script>

<style scoped>

.program-actions {
    display: flex;
    gap: 4px;
    max-height: 0;
    margin-top: 0;
    visibility: hidden;
    transition: max-height 0s, margin-top 0s, visibility 0s;
}

.program-item:hover .program-actions {
    max-height: 24px;
    margin-top: 2px;
    visibility: visible;
    transition-delay: 0.5s;
}

.program-action-button {
    height: 24px !important;
    min-width: 80px !important;
    padding: 0 12px !important;
}

.program-action-button-record {
    background-color: var(--tt-record-button, #f08080f0) !important;
    color: var(--app-text, #323232) !important;
}

.program-action-button-search {
    background-color: var(--tt-action-button, #a0c0f0f0) !important;
    color: var(--app-text, #323232) !important;
}

.program-action-button-reserved {
    background-color: rgba(220, 80, 80, 0.80) !important;
    color: #fff !important;
}

.program-action-button-watch {
    background-color: var(--tt-watch-button, #f5d77af0) !important;
    color: var(--app-text, #323232) !important;
}

.program-jump-buttons {
    position: sticky;
    top: 0;
    z-index: 30;
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 4px;
    padding: 6px;
    background-color: var(--app-surface, #fff);
}

.program-jump-button {
    min-width: 0 !important;
    padding: 0 4px !important;
}

.program-section,
.program-subsection {
    overflow: visible;
}

.program-header {
    position: sticky;
    top: var(--program-jump-offset, 0);
    z-index: 20;
    padding: 2px 8px;
    min-height: 32px;
    height: auto;
    color: #f0f0f0;
    background-color: var(--app-section-header, #9da5d3);
    background-image: linear-gradient(to right, transparent 75%, rgba(0, 0, 0, 0.12) 100%);
    box-shadow: 0 1px 0 rgba(0, 0, 0, 0.12);
}

.program-subheader {
    position: sticky;
    top: var(--program-subheader-offset, 32px);
    z-index: 10;
    padding: 0 16px;
    min-height: 30px;
    height: auto;
    box-shadow: 0 1px 0 rgba(0, 0, 0, 0.08);
}

</style>
